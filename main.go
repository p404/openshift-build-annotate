package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
	"html"

	"github.com/Jeffail/gabs"
	"github.com/google/go-containerregistry/pkg/crane"
)

type OpenShiftLabels struct {
	commitAuthor string
	commitDate string
	commitId string
	commitMessage string
	commitRef string
}

// labelStrcut, _ := GetLabelsFromImage("quay.io/bitnami/mysql")
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

	if data.Exists("config", "Labels") {
		if data.Exists("config", "Labels", "maintainer") {
			ocl.commitAuthor = data.Path("config.Labels.maintainer").Data().(string)
		}
	} else {
		return nil, fmt.Errorf("Could not get labels from manifest image registry %s: %w", image, err)
	}

	return ocl, nil
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "hello %q", html.EscapeString(r.URL.Path))
}

func main(){
	log.Println("Starting server openshift-build-annotate...")

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)

	s := &http.Server{
		Addr:           ":8443",
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1048576
	}

	log.Fatal(s.ListenAndServeTLS("./ssl/mutateme.pem", "./ssl/mutateme.key"))
}