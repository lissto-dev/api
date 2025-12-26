package metadata

import (
	"encoding/json"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const keyTimestampsAnnotation = "lissto.dev/kt"

// GetKeyTimestamps parses the key timestamps annotation from a Kubernetes object
func GetKeyTimestamps(obj metav1.Object) map[string]int64 {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return make(map[string]int64)
	}

	data := annotations[keyTimestampsAnnotation]
	if data == "" {
		return make(map[string]int64)
	}

	timestamps := make(map[string]int64)
	if err := json.Unmarshal([]byte(data), &timestamps); err != nil {
		return make(map[string]int64)
	}

	return timestamps
}

// UpdateKeyTimestamps updates the timestamps for the given keys on a Kubernetes object
func UpdateKeyTimestamps(obj metav1.Object, keys []string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
		obj.SetAnnotations(annotations)
	}

	timestamps := GetKeyTimestamps(obj)
	now := time.Now().Unix()

	for _, key := range keys {
		timestamps[key] = now
	}

	data, _ := json.Marshal(timestamps)
	annotations[keyTimestampsAnnotation] = string(data)
	obj.SetAnnotations(annotations)
}
