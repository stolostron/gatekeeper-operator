package controllers

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var errCrdNotReady = errors.New("CRD is not ready")

// Check CRD status is "True" and type is "NamesAccepted"
func checkCrdAvailable(ctx context.Context, dynamicClient *dynamic.DynamicClient, resourceName, crdName string) (bool, error) {
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	crd, err := dynamicClient.Resource(crdGVR).
		Get(ctx, crdName, metav1.GetOptions{})
	if err != nil {
		setupLog.V(1).Info(fmt.Sprintf("Cannot fetch %s CRD", resourceName))

		return false, err
	}

	conditions, ok, _ := unstructured.NestedSlice(crd.Object, "status", "conditions")
	if !ok {
		setupLog.V(1).Info(fmt.Sprintf("Cannot parse %s status conditions", resourceName))

		return false, errors.New("Failed to parse status, conditions")
	}

	for _, condition := range conditions {
		parsedCondition := condition.(map[string]interface{})

		status, ok := parsedCondition["status"].(string)
		if !ok {
			setupLog.V(1).Info(fmt.Sprintf("Cannot parse %s conditions status", resourceName))

			return false, errors.New("Failed to parse status string")
		}

		conditionType, ok := parsedCondition["type"].(string)
		if !ok {
			setupLog.V(1).Info(fmt.Sprintf("Cannot parse %s conditions type", resourceName))

			return false, errors.New(fmt.Sprintf("Failed to parse %s conditions type", resourceName))
		}

		if conditionType == "NamesAccepted" && status == "True" {
			setupLog.V(1).Info("The CRD is ready", "CRD", crdName)

			return true, nil
		}
	}

	setupLog.V(1).Info("The CRD is not ready yet", "CRD", crdName)

	return false, nil
}
