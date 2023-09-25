package controllers

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

var ErrNotFoundDiscovery = errors.New("there are no matched apiGroup, version or kind")

type DiscoveryStorage struct {
	apiResourceList        []*metav1.APIResourceList
	discoveryLastRefreshed time.Time
	ClientSet              *kubernetes.Clientset
	Log                    logr.Logger
}

func (r *DiscoveryStorage) getSyncOnlys(constraintMatchKinds []interface{}) (
	[]v1alpha1.SyncOnlyEntry, error,
) {
	syncOnlys := []v1alpha1.SyncOnlyEntry{}

	var finalErr error

	for _, match := range constraintMatchKinds {
		newKind, ok := match.(map[string]interface{})
		if !ok {
			continue
		}

		apiGroups, ok := newKind["apiGroups"].([]interface{})
		if !ok {
			continue
		}

		kindsInKinds, ok := newKind["kinds"].([]interface{})
		if !ok {
			continue
		}

		for _, apiGroup := range apiGroups {
			for _, kind := range kindsInKinds {
				version, err := r.getAPIVersion(kind.(string), apiGroup.(string), false, r.ClientSet)
				if err != nil {
					r.Log.V(1).Info("getAPIVersion has error but continue")

					if finalErr == nil {
						finalErr = err
					} else {
						// Accumulate error
						finalErr = fmt.Errorf("%w; %w", finalErr, err)
					}

					continue
				}

				syncOnlys = append(syncOnlys, v1alpha1.SyncOnlyEntry{
					Group:   apiGroup.(string),
					Version: version,
					Kind:    kind.(string),
				})
			}
		}
	}

	return syncOnlys, finalErr
}

// getAPIVersion gets the server preferred API version for the constraint's match kind entry
// Constraint only provide kind and apiGroup. However the config resource need version
func (r *DiscoveryStorage) getAPIVersion(kind string,
	apiGroup string, skipRefresh bool, clientSet *kubernetes.Clientset,
) (string, error) {
	// Cool time(10 min) to refresh discoveries
	if len(r.apiResourceList) == 0 ||
		r.discoveryLastRefreshed.Add(time.Minute*10).Before(time.Now()) {
		err := r.refreshDiscoveryInfo()
		if err != nil {
			return "", err
		}

		// The discovery is just refeshed so skip another refesh
		skipRefresh = true
	}

	for _, resc := range r.apiResourceList {
		groupVerison, err := schema.ParseGroupVersion(resc.GroupVersion)
		if err != nil {
			r.Log.Error(err, "Cannot parse the group and version in getApiVersion ", "GroupVersion:", resc.GroupVersion)

			continue
		}

		group := groupVerison.Group
		version := groupVerison.Version
		// Consider groupversion == v1 or groupversion == app1/v1
		for _, apiResource := range resc.APIResources {
			if apiResource.Kind == kind && group == apiGroup {
				return version, nil
			}
		}
	}

	if !skipRefresh {
		// Get new discoveryInfo, when any resource is not found
		err := r.refreshDiscoveryInfo()
		if err != nil {
			return "", err
		}

		// Retry one more time after refresh the discovery
		return r.getAPIVersion(kind, apiGroup, true, clientSet)
	}

	return "", ErrNotFoundDiscovery
}

// Retrieve all groups and versions to add in config sync
// Constraints present only kind and group so this function helps to find the version
func (r *DiscoveryStorage) refreshDiscoveryInfo() error {
	r.discoveryLastRefreshed = time.Now()

	discoveryClient := r.ClientSet.Discovery()

	apiList, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return err
	}

	// Save fetched discovery at apiResourceList
	r.apiResourceList = apiList

	return nil
}
