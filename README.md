# Mutating webhook example for LSF-K8S Integration

This mutatating webhook
[admission controller](https://kubernetes.io/docs/admin/admission-controllers).
can be used to automatically inject pod parameters to allow scheduling of pods through the 
[Spectrum LSF-Kuberentes integration ] ( https://github.com/IBMSpectrumComputing/lsf-kubernetes/)


This is a toy example. It is based on the examples in [caesarxuchao/example-webhook-admission-controller](https://github.com/caesarxuchao/example-webhook-admission-controller) and
[Istio's webhook](https://github.com/istio/istio/blob/master/pilot/pkg/kube/inject/webhook.go).
(Here's an simpler example of a controller from [Kelsey](https://github.com/kelseyhightower/denyenv-validating-admission-webhook)).

## What this example webhook does

In `mutating-webhook/main.go` we define a mutating webhook server that accepts requests from the k8s apiserver.
It processes the request and always accepts the request for admission.
In addition, it modifies any pod specs to include the LSF schedulerName and adds annotation for the fairshare group  based environement variables that are passed into the pod. The environment variables control which namespaces should be allowed to use the LSF scheduler and which namespaces are assigned to gold,silver,bronze classes. More complicated logic can be built into this control to set other annotations that control the scheduling behaviour (e.g GPU resources, project or application settings).


## Running the webhook in  IBM Cloud Private

```
# deploy the webhook
kubectl apply -n kube-system -f https://raw.githubusercontent.com/khahmed/admission-webhook-example/master/mutating-webhook/k8s/webhook-server.yaml 

# ensure the webhook was created properly (wait 10 seconds)
kubectl describe MutatingWebhookConfiguration

# optionally tail the webhook logs
kubectl logs -f $(kubectl get po -o jsonpath='{.items[0].metadata.name}')


# look for our injected annotation
kubectl describe po
kubectl get po $(kubectl get po -o jsonpath='{.items[1].metadata.name}') -o jsonpath='{.metadata.annotations}'
```

## Cleanup
Delete the webhook deployment as well as the webhook config:
```
./cleanup
```

```
