package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/mattbaird/jsonpatch"

	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()

	// TODO(https://github.com/kubernetes/kubernetes/issues/57982)
	defaulter = runtime.ObjectDefaulter(runtimeScheme)

	lsfAnnotation = map[string]string{
		//"lsf.ibm.com/project":               "project-1",
		//"lsf.ibm.com/application":    "" ,
		//"lsf.ibm.com/gpu": "0",
		//"lsf.ibm.com/queue": "normal",
		//"lsf.ibm.com/jobGroup": "normal",
		//"lsf.ibm.com/fairshareGroup": "normal",
		//"lsf.ibm.com/user": "normal",
	}
)

// the Path of the JSON patch is a JSON pointer value
// so we need to escape any "/"s in the key we add to the annotation
// https://tools.ietf.org/html/rfc6901
func escapeJSONPointer(s string) string {
	esc := strings.Replace(s, "~", "~0", -1)
	esc = strings.Replace(esc, "/", "~1", -1)
	return esc
}

var kubeSystemNamespaces = []string{
	metav1.NamespaceSystem,
	metav1.NamespacePublic,
	"default",
	"istio-system",
	"cert-manager",
}

var allowedNamespaces = []string{}
var goldNamespaces = []string{}
var silverNamespaces = []string{}
var bronzeNamespaces = []string{}

var mapNsNameToFsGroup = true
var mapUserFileSystemInfo = false

type SecurityContext  struct {
   runAsUser, runAsGroup, fsGroup int
}

var m  map[string]SecurityContext

func handler(w http.ResponseWriter, r *http.Request) {
	log.Println("Handling a request")

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("error: %v", err)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Printf("Wrong content type. Got: %s", contentType)
		return
	}

	admReq := v1beta1.AdmissionReview{}
	admResp := v1beta1.AdmissionReview{}

	if _, _, err := deserializer.Decode(body, nil, &admReq); err != nil {
		log.Printf("Could not decode body: %v", err)
		admResp.Response = admissionError(err)
	} else {
		admResp.Response = getAdmissionDecision(&admReq)
	}

	resp, err := json.Marshal(admResp)
	if err != nil {
		log.Printf("error marshalling decision: %v", err)
	}
	log.Printf(string(resp))
	if _, err := w.Write(resp); err != nil {
		log.Printf("error writing response %v", err)
	}
}

func admissionError(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{Message: err.Error()},
	}
}

func getAdmissionDecision(admReq *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	req := admReq.Request
	var pod corev1.Pod

	err := json.Unmarshal(req.Object.Raw, &pod)
	if err != nil {
		log.Printf("Could not unmarshal raw object: %v", err)
		return admissionError(err)
	}

	log.Printf("AdmissionReview for Kind=%v Namespace=%v Name=%v UID=%v Operation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, req.UID, req.Operation, req.UserInfo)
	//log.Printf("Pod=%v", pod)
	log.Printf("Pod.Annotations=%v", pod.Annotations)
	//log.Printf("Pod.Labels=%v", pod.Labels)

	//log.Printf("calling shouldInject for pod  %s %s objectMeta.Namespace=%s", pod.Namespace, pod.Name, pod.ObjectMeta.Namespace)
	if !shouldInject(req.Namespace) {
		log.Printf("Skipping inject for %s %s", req.Namespace, req.Name)
		return &v1beta1.AdmissionResponse{
			Allowed: true,
			UID:     req.UID,
		}
	}

	/*
	   if req.Namespace  ==  "proj-1"  {
	        lsfAnnotation["lsf.ibm.com/fairshareGroup"] = "gold"
	   } else if req.Namespace == "proj-2"  {
	        lsfAnnotation["lsf.ibm.com/fairshareGroup"] = "silver"
	   } else  {
	        lsfAnnotation["lsf.ibm.com/fairshareGroup"] = "bronze"
	   }
	*/
	lsfAnnotation["lsf.ibm.com/fairshareGroup"] = getServiceClass(req.Namespace)

	patch, err := patchConfig(&pod, lsfAnnotation)

	if err != nil {
		log.Printf("Error creating lsf patch: %v", err)
		return admissionError(err)
	}

	jsonPatchType := v1beta1.PatchTypeJSONPatch

	return &v1beta1.AdmissionResponse{
		Allowed:   true,
		Patch:     patch,
		PatchType: &jsonPatchType,
		UID:       req.UID,
	}
}

func patchConfig(pod *corev1.Pod, annotations map[string]string) ([]byte, error) {
	var patch []jsonpatch.JsonPatchOperation

	patch = append(patch, addAnnotations(pod.Annotations, annotations)...)
	op := "add"
	patch = append(patch, jsonpatch.JsonPatchOperation{
		Operation: op,
		Path:      "/spec/schedulerName",
		Value:     "lsf",
	})

        user :=  annotations["lsf.ibm.com/user"]
	log.Printf("Looking up if user %s in table", user )
        val, ok  := m[user]
        if ok {
	   log.Printf("Found  user %s %s", user, mapUserFileSystemInfo )
        }
        if  user !=  "" && mapUserFileSystemInfo  && ok {
            op = "add"
	    log.Printf("Patching runAsUser %s", val.runAsUser)
	    patch = append(patch, jsonpatch.JsonPatchOperation{
		Operation: op,
		Path:      "/spec/securityContext/runAsUser",
		Value:     val.runAsUser,
	    })
            op = "add"
	    log.Printf("Patching runAsGroup %s", val.runAsGroup)
	    patch = append(patch, jsonpatch.JsonPatchOperation{
		Operation: op,
		Path:      "/spec/securityContext/runAsGroup",
		Value:     val.runAsGroup,
	    })
            op = "add"
	    patch = append(patch, jsonpatch.JsonPatchOperation{
		Operation: op,
		Path:      "/spec/securityContext/fsGroup",
		Value:     val.fsGroup,
	    })
        
        }
	return json.Marshal(patch)
}

func addAnnotations(current map[string]string, toAdd map[string]string) []jsonpatch.JsonPatchOperation {
	var patch []jsonpatch.JsonPatchOperation

	for key, val := range toAdd {
		if current == nil {
			current = map[string]string{}
			patch = append(patch, jsonpatch.JsonPatchOperation{
				Operation: "add",
				Path:      "/metadata/annotations",
				Value: map[string]string{
					key: val,
				},
			})
		} else {
			op := "add"
			if current[key] != "" {
				op = "replace"
			}
			patch = append(patch, jsonpatch.JsonPatchOperation{
				Operation: op,
				Path:      "/metadata/annotations/" + escapeJSONPointer(key),
				Value:     val,
			})
		}
	}

	return patch
}

func shouldInject(namespace string) bool {
	shouldInject := true

	// don't attempt to inject pods in the Kubernetes system namespaces
	for _, ns := range kubeSystemNamespaces {
		//log.Printf("Checking inject for %s %s", ns,  namespace)
		if namespace == ns {
			shouldInject = false
			break
		}
	}

	if len(allowedNamespaces) > 0 {
		shouldInject = false
		for _, ns := range allowedNamespaces {
			log.Printf("Checking inject for allowed namespace %s %s", ns, namespace)
			if namespace == ns {
				shouldInject = true
			}
		}
	}

	return shouldInject
}

func getServiceClass(namespace string) string {
	// For hiearchical FS we just return the namespace name as the FS groupname
	if mapNsNameToFsGroup == true {
		return namespace
	}
	for _, ns := range goldNamespaces {
		if namespace == ns {
			return "gold"
		}
	}
	for _, ns := range silverNamespaces {
		if namespace == ns {
			return "silver"
		}
	}
	for _, ns := range bronzeNamespaces {
		if namespace == ns {
			return "bronze"
		}
	}
	return "bronze"
}

func main() {
	addr := flag.String("addr", ":8080", "address to serve on")

	allowed := os.Getenv("ALLOWED_NAMESPACES")
	if allowed != "" {
		allowedNamespaces = strings.Fields(allowed)
	}
	gold := os.Getenv("GOLD_NAMESPACES")
	if gold != "" {
		goldNamespaces = strings.Fields(gold)
	}
	silver := os.Getenv("SILVER_NAMESPACES")
	if silver != "" {
		silverNamespaces = strings.Fields(silver)
	}
	bronze := os.Getenv("BRONZE_NAMESPACES")
	if bronze != "" {
		bronzeNamespaces = strings.Fields(bronze)
	}
	mapAllowedNsNameToFSGroup := os.Getenv("MAP_NS_TO_FSGROUP")
	if mapAllowedNsNameToFSGroup != "" || mapAllowedNsNameToFSGroup == "N" {
		mapNsNameToFsGroup = false
	} else {
		mapNsNameToFsGroup = true
	}
	injectFileSystemInfo := os.Getenv("INJECT_FILESYSTEM_INFO")
	if injectFileSystemInfo != "" &&  injectFileSystemInfo == "N" {
             mapUserFileSystemInfo = false
        } else {
             mapUserFileSystemInfo = true
             // Just create a dummy map for test
	     log.Printf("Creating user security context map" )
             m = make(map[string]SecurityContext)
             m["joe"] = SecurityContext{1000,1000,1000,}
             m["ann"] = SecurityContext{1001,1001,2000,}
             m["jim"] = SecurityContext{1002,1002,3000,}
        }

        
	http.HandleFunc("/", handler)

	flag.CommandLine.Parse([]string{}) // hack fix for https://github.com/kubernetes/kubernetes/issues/17162

	log.Printf("Starting HTTPS webhook server on %+v", *addr)
	clientset := getClient()
	server := &http.Server{
		Addr:      *addr,
		TLSConfig: configTLS(clientset),
	}
	go selfRegistration(clientset, caCert)
	server.ListenAndServeTLS("", "")
}
