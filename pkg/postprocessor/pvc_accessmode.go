package postprocessor

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// PVCAccessModeNormalizer ensures all PVCs use ReadWriteOnce accessMode
// This is needed because Kompose may generate ReadOnlyMany for volumes with :ro flag
type PVCAccessModeNormalizer struct{}

func NewPVCAccessModeNormalizer() *PVCAccessModeNormalizer {
	return &PVCAccessModeNormalizer{}
}

// NormalizeAccessModes changes all PVC accessModes to ReadWriteOnce
func (p *PVCAccessModeNormalizer) NormalizeAccessModes(objects []runtime.Object) []runtime.Object {
	for i, obj := range objects {
		if pvc, ok := obj.(*corev1.PersistentVolumeClaim); ok {
			// Force ReadWriteOnce accessMode for all PVCs
			pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			}
			objects[i] = pvc
		}
	}
	return objects
}
