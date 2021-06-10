package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/golang/glog"
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

var ignoredNamespaces = []string{
	metav1.NamespaceSystem,
	metav1.NamespacePublic,
}

const (
	admissionWebhookAnnotationInjectKey = "openshift-build-annotate/mutate"
	admissionWebhookAnnotationStatusKey = "openshift-build-annotate/status"
)

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
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

func mutationRequired(ignoredList []string, metadata *metav1.ObjectMeta) bool {
	for _, namespace := range ignoredList {
		if metadata.Namespace == namespace {
			glog.Infof("Skip mutation for %v for it's in special namespace:%v", metadata.Name, metadata.Namespace)
			return false
		}
	}

	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	status := annotations[admissionWebhookAnnotationStatusKey]

	var required bool
	if strings.ToLower(status) == "injected" {
		required = false
	} else {
		switch strings.ToLower(annotations[admissionWebhookAnnotationInjectKey]) {
		default:
			required = true
		case "n", "no", "false", "of":
			required = false
		}
	}

	glog.Infof("Mutation policy for %v/%v: status: %q required:%v", metadata.Namespace, metadata.Name, status, required)
	return required
}

func updateAnnotation(target map[string]string, added map[string]string) (patch []patchOperation) {
	for key, value := range added {
		if target == nil || target[key] == "" {
			target = map[string]string{}
			patch = append(patch, patchOperation{
				Op:   "add",
				Path: "/metadata/annotations",
				Value: map[string]string{
					key: value,
				},
			})
		} else {
			patch = append(patch, patchOperation{
				Op:    "replace",
				Path:  "/metadata/annotations/" + key,
				Value: value,
			})
		}
	}
	return patch
}

func createPatch(pod *corev1.Pod, annotations map[string]string) ([]byte, error) {
	var patch []patchOperation

	patch = append(patch, updateAnnotation(pod.Annotations, annotations)...)

	return json.Marshal(patch)
}

func Mutate(body []byte) ([]byte, error) {
	responseBody := []byte{}
	var err error

	admReview := v1beta1.AdmissionReview{}
	if err := json.Unmarshal(body, &admReview); err != nil {
		return nil, fmt.Errorf("unmarshaling request failed with %s", err)
	}

	admReview.Response = MutateResponse(&admReview)
	admReview.Response.UID = admReview.Request.UID
	responseBody, err = json.Marshal(admReview)
	if err != nil {
		return nil, err
	}
	return responseBody, nil
}

func MutateResponse(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	req := ar.Request
	glog.Infof("mutate Request:%v", req)
	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		glog.Errorf("Could not unmarshal raw object: %v", err)
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	glog.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v (%v) UID=%v patchOperation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, pod.Name, req.UID, req.Operation, req.UserInfo)

	if !mutationRequired(ignoredNamespaces, &pod.ObjectMeta) {
		glog.Infof("Skipping mutation for %s/%s due to policy check", pod.Namespace, pod.Name)
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	annotations := map[string]string{admissionWebhookAnnotationStatusKey: "injected"}
	patchBytes, err := createPatch(&pod, annotations)
	if err != nil {
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	glog.Infof("AdmissionResponse: patch=%v\n", string(patchBytes))
	return &v1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *v1beta1.PatchType {
			pt := v1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
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
