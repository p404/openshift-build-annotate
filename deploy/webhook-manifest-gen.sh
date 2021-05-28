#!/bin/bash

kubectl delete MutatingWebhookConfiguration openshift-build-annotate 2>/dev/null || true
# create server webhook configuration and send to k8s API
cat <<EOF | kubectl create -f -
apiVersion: admissionregistration.k8s.io/v1beta1
kind: MutatingWebhookConfiguration
metadata:
  name: openshift-build-annotate
  labels:
    app: openshift-build-annotate
webhooks:
  - name: openshift-build-annotate.default.svc.cluster.local
    clientConfig:
      caBundle: $(kubectl get configmap -n kube-system extension-apiserver-authentication -o=jsonpath='{.data.client-ca-file}' | base64 | tr -d '\n')
      service:
        name: openshift-build-annotate
        namespace: default
        path: "/mutate"
        port: 443
    rules:
      - operations: ["CREATE"]
        apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
    sideEffects: None
    timeoutSeconds: 5
    reinvocationPolicy: Never
    failurePolicy: Ignore
EOF