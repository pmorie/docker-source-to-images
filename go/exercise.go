package sti

import (
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/fsouza/go-dockerclient"
)

// Describes a request to execise an image in a reusable test harness
type ExerciseRequest struct {
	BuildRequest
	Incremental bool
	Port        string
	Path        string
	Title       string
}

type ExerciseResult STIResult

var htmlTitleExp = regexp.MustCompile(`<title>([^<]+)</title>`)

// Service the supplied ValidateRequest and return a ValidateResult.
func Exercise(req ExerciseRequest) (*ExerciseResult, error) {
	h, err := newHandler(req.Request)
	if err != nil {
		return nil, err
	}

	result := &ExerciseResult{Success: true}

	_, err = Build(req.BuildRequest)
	if err != nil {
		return nil, err
	}

	_, err = h.checkAndPull(req.Tag)
	if err != nil {
		return nil, err
	}

	config := docker.Config{Image: req.Tag, AttachStdout: true, AttachStderr: true}
	container, err := h.dockerClient.CreateContainer(docker.CreateContainerOptions{"", &config})
	if err != nil {
		return nil, err
	}

	hostConfig := docker.HostConfig{PublishAllPorts: true}
	err = h.dockerClient.StartContainer(container.ID, &hostConfig)
	if err != nil {
		return nil, err
	}

	container, err = h.dockerClient.InspectContainer(container.ID)
	if err != nil {
		return nil, err
	}

	mappedPort := container.NetworkSettings.Ports[docker.Port(req.Port)]
	log.Printf("Container has port %s mapped to %s:%s\n", req.Port, mappedPort[0].HostIp, mappedPort[0].HostPort)

	url := "http://" + mappedPort[0].HostIp + ":" + mappedPort[0].HostPort
	if req.Path != "" {
		if strings.HasPrefix(req.Path, "/") {
			url += req.Path
		} else {
			url += "/" + req.Path
		}
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	matches := htmlTitleExp.FindAllStringSubmatch(string(body), -1)
	if len(matches) == 0 {
		if h.verbose {
			log.Println("Response did not contain a title")
		}
	}

	responseTitle := matches[0][1]
	if responseTitle != req.Title {
		result.Success = false
		result.Messages = append(result.Messages, "Title did not match")
	}

	_ = h.dockerClient.KillContainer(docker.KillContainerOptions{ID: container.ID})

	return result, nil
}
