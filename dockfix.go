package dockfix

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/wrapp/env"
)

var dockerURL string

// NewClient returns a new docker client, with handling of DOCKER_HOST
// and DOCKER_CERT_PATH
func NewClient() (*docker.Client, error) {

	dockerURL = env.Default("DOCKER_HOST", "unix:///var/run/docker.sock")
	dockerCertPath := env.Default("DOCKER_CERT_PATH", "")

	if dockerCertPath != "" {
		ca := path.Join(dockerCertPath, "ca.pem")
		key := path.Join(dockerCertPath, "key.pem")
		cert := path.Join(dockerCertPath, "cert.pem")

		return docker.NewTLSClient(dockerURL, cert, key, ca)
	}
	return docker.NewClient(dockerURL)
}

// Container is a wrapper around a docker.Container
type Container struct {
	*docker.Container
	name string
}

// PortURL returns a URL to the first specified port matching portSpec
// It also substitutes the host flow DOCKER_HOST if applicable
func PortURL(cont Container, portSpec docker.Port) (*url.URL, error) {
	port := cont.NetworkSettings.Ports[portSpec][0]
	var host string
	envHost := os.Getenv("DOCKER_HOST")
	if envHost != "" {
		envHostURL, err := url.Parse(envHost)
		if err != nil {
			return nil, err
		}
		host = strings.Split(envHostURL.Host, ":")[0]
	} else {
		host = string(port.HostIP)
	}
	return &url.URL{
		Scheme: portSpec.Proto(),
		Host:   fmt.Sprintf("%v:%v", host, port.HostPort),
	}, nil
}

// StartContainer starts a container with the specified base image, creating one
// if necessary. The container id is stored in a file named <name>.container.
func StartContainer(name, baseImage string) (Container, error) {
	c := Container{name: name}
	dc, err := NewClient()
	if err != nil {
		return c, err
	}

	containerFileName := name + ".container"
	cid, _ := ioutil.ReadFile(containerFileName)
	var containerID string
	if len(cid) != 0 {
		log.Print("Using existing container: ", string(cid))
		containerID = string(cid)
	} else {
		log.Print("Creating new container for ", baseImage)
		cont, err := dc.CreateContainer(
			docker.CreateContainerOptions{
				Config: &docker.Config{
					Image: baseImage,
				},
			},
		)
		if err != nil {
			return c, err
		}
		log.Print("Created container: ", string(cont.ID))
		containerID = cont.ID
	}
	ioutil.WriteFile(containerFileName, []byte(containerID), 0644)
	hc := docker.HostConfig{
		PublishAllPorts: true,
	}
	// Error intentionally ignored, it is ok if the container is already running,
	// and if we run into other problems, InspectContainer will report it
	dc.StartContainer(containerID, &hc)
	cont, err := dc.InspectContainer(containerID)
	if err != nil {
		return c, err
	}

	c.Container = cont

	return c, nil
}

// StopContainer stops the running container.
func StopContainer(c Container) {
	dc, _ := NewClient()
	dc.KillContainer(docker.KillContainerOptions{
		ID: c.ID,
	})
}

// RemoveContainer removes the container and its id file.
func RemoveContainer(c Container) error {
	dc, _ := NewClient()
	err := dc.RemoveContainer(docker.RemoveContainerOptions{
		ID: c.ID,
	})
	if err != nil {
		return err
	}

	containerFileName := c.name + ".container"
	return os.Remove(containerFileName)
}
