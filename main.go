package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/google/go-containerregistry/pkg/crane"
	v1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type OpenShiftLabels struct {
	commitAuthor  string
	commitDate    string
	commitId      string
	commitMessage string
	commitRef     string
}

// labelStrcut, _ := GetLabelsFromImage("quay.io/bitnami/mysql")
func GetLabelsFromImage(image string) (*OpenShiftLabels, error) {
	ocl := new(OpenShiftLabels)

	res, err := crane.Config(image)
	if err != nil {
		return nil, fmt.Errorf("could not get defaults about image %s: %w", image, err)
	}
	data, err := gabs.ParseJSON(res)
	if err != nil {
		return nil, fmt.Errorf("could not parse response from registry for image %s: %w", image, err)
	}

	if data.Exists("config", "Labels") {
		if data.Exists("config", "Labels", "maintainer") {
			ocl.commitAuthor = data.Path("config.Labels.maintainer").Data().(string)
		}
	} else {
		return nil, fmt.Errorf("could not get labels from manifest image registry %s: %w", image, err)
	}

	return ocl, nil
}

func sendError(err error, w http.ResponseWriter) {
	log.Println(err)
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "%s", err)
}

func Mutate(body []byte) ([]byte, error) {
	var pod corev1.Pod
	var err error
	responseBody := []byte{}

	admReview := v1beta1.AdmissionReview{}
	if err := json.Unmarshal(body, &admReview); err != nil {
		return nil, fmt.Errorf("unmarshaling request failed with %s", err)
	}
	ar := admReview.Request

	if err := json.Unmarshal(ar.Object.Raw, &pod); err != nil {
		log.Printf("could not unmarshal raw object: %v", err)
	}

	log.Printf("AdmissionReview for Kind=%v, Namespace=%v Name=%v (%v) UID=%v patchOperation=%v UserInfo=%v", ar.Kind, ar.Namespace, ar.Name, ar.Name, ar.UID, ar.Operation, ar.UserInfo)

	resp := v1beta1.AdmissionResponse{}
	resp.Allowed = true
	resp.UID = ar.UID

	resp.Result = &metav1.Status{
		Status: "Success",
	}

	admReview.Response = &resp
	responseBody, err = json.Marshal(admReview)
	if err != nil {
		return nil, err
	}

	return responseBody, nil
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "hello %q", html.EscapeString(r.URL.Path))
}

func handleMutate(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()

	if err != nil {
		sendError(err, w)
		return
	}

	mutated, err := Mutate(body)
	if err != nil {
		sendError(err, w)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(mutated)
}

func main() {
	log.Println("Starting server openshift-build-annotate...")

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/mutate", handleMutate)

	s := &http.Server{
		Addr:           ":8443",
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1048576
	}

	log.Fatal(s.ListenAndServeTLS("./tls/tls.crt", "./tls/tls.key"))
}
