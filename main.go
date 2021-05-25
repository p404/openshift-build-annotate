package main

import (
	"fmt"

	"github.com/Jeffail/gabs"
	"github.com/google/go-containerregistry/pkg/crane"
)

type OpenShiftLabels struct {
	commitAuthor []string
	commitDate []string
	commitId []string
	commitMessage [] string
	commitRef [] string
}


func GetLabelsFromImage(image string) (*OpenShiftLabels, error) {
	ocl := new(OpenShiftLabels)

	res, err := crane.Config(image)
	if err != nil {
		return nil, fmt.Errorf("Could not get defaults about image %s: %w", image, err)
	}
	data, err := gabs.ParseJSON(res)
	if err != nil {
		return nil, fmt.Errorf("Could not parse response from registry for image %s: %w", image, err)
	}

	fmt.Println(data.Path("config.Labels"))

	return ocl, nil
}

func main(){
	GetLabelsFromImage("quay.io/bitnami/mysql")
}