package controllers

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	"github.com/open-policy-agent/gatekeeper/v3/pkg/wildcard"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1alpha1 "github.com/gatekeeper/gatekeeper-operator/api/v1alpha1"
)

func TestAddDefaultExemptNamespaces(t *testing.T) {
	reconciler := ConfigReconciler{
		Scheme: runtime.NewScheme(),
		Client: fake.NewClientBuilder().Build(),
	}
	g := NewWithT(t)
	ctx := context.TODO()

	config := &v1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "config",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "gatekeeper", UID: "test-1"},
			},
		},
		Spec: v1alpha1.ConfigSpec{
			Match: []v1alpha1.MatchEntry{
				{
					ExcludedNamespaces: []wildcard.Wildcard{
						"dog-bird", "tiger-ns",
					},
					Processes: []string{
						"audit", "webhook", "sync",
					},
				},
			},
		},
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		Spec: operatorv1alpha1.GatekeeperSpec{
			Config: nil,
		},
	}

	defaultConfig := getDefaultConfig("test")

	disableDefaultMatches := true

	t.Run(
		"Should update the existing config when the existing config has match and gatekeeper match is []",
		func(t *testing.T,
		) {
			gatekeeper = &operatorv1alpha1.Gatekeeper{
				ObjectMeta: metav1.ObjectMeta{
					UID: "test-1",
				},
				Spec: operatorv1alpha1.GatekeeperSpec{
					Config: &operatorv1alpha1.ConfigConfig{
						Matches: []v1alpha1.MatchEntry{},
					},
				},
			}

			_ = reconciler.setExemptNamespaces(ctx, config, gatekeeper)
			g.Expect(config.Spec.Match).Should(HaveLen(1))
			g.Expect(config.Spec.Match).Should(BeComparableTo(defaultConfig.Spec.Match))
		})

	t.Run("Should update the existing config when the existing config is nil", func(t *testing.T) {
		gatekeeper = &operatorv1alpha1.Gatekeeper{
			ObjectMeta: metav1.ObjectMeta{
				UID: "test-1",
			},
			Spec: operatorv1alpha1.GatekeeperSpec{},
		}

		_ = reconciler.setExemptNamespaces(ctx, config, gatekeeper)
		g.Expect(config.Spec.Match).Should(HaveLen(1))
		g.Expect(config.Spec.Match).Should(BeComparableTo(defaultConfig.Spec.Match))
	})

	t.Run("Should override existing config match when gatekeeper match is not empty", func(t *testing.T) {
		gatekeeper = &operatorv1alpha1.Gatekeeper{
			ObjectMeta: metav1.ObjectMeta{
				UID: "test-1",
			},
			Spec: operatorv1alpha1.GatekeeperSpec{
				Config: &operatorv1alpha1.ConfigConfig{
					Matches: []v1alpha1.MatchEntry{
						{
							ExcludedNamespaces: []wildcard.Wildcard{
								"cat", "cat1",
							},
							Processes: []string{
								"audit",
							},
						},
					},
				},
			},
		}

		config = &v1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "config",
			},
			Spec: v1alpha1.ConfigSpec{
				Match: []v1alpha1.MatchEntry{},
			},
		}

		_ = reconciler.setExemptNamespaces(ctx, config, gatekeeper)
		g.Expect(config.Spec.Match).Should(HaveLen(2))
		g.Expect(config.Spec.Match[0]).Should(BeComparableTo(defaultConfig.Spec.Match[0]))
		g.Expect(config.Spec.Match[1]).Should(BeComparableTo(gatekeeper.Spec.Config.Matches[0]))
	})

	t.Run("Should not append default exempt namespaces match when DisableDefaultMatches is true", func(t *testing.T) {
		config = &v1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "config",
			},
			// Config Match is empty. Config will be updated with gatekeeper.spec.config.matches
			Spec: v1alpha1.ConfigSpec{},
		}

		gatekeeper = &operatorv1alpha1.Gatekeeper{
			ObjectMeta: metav1.ObjectMeta{
				UID: "test-1",
			},
			Spec: operatorv1alpha1.GatekeeperSpec{
				Config: &operatorv1alpha1.ConfigConfig{
					Matches: []v1alpha1.MatchEntry{{
						ExcludedNamespaces: []wildcard.Wildcard{
							"cat-ns", "dog-ns",
						},
						Processes: []string{
							"audit", "webhook", "sync",
						},
					}},
					DisableDefaultMatches: disableDefaultMatches,
				},
			},
		}

		_ = reconciler.setExemptNamespaces(ctx, config, gatekeeper)
		g.Expect(config.Spec.Match).Should(Equal(gatekeeper.Spec.Config.Matches))
	})

	t.Run("Should append existing config match when DisableDefaultMatches is not provided", func(t *testing.T) {
		config = &v1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "config",
			},
			Spec: v1alpha1.ConfigSpec{},
		}

		gatekeeper = &operatorv1alpha1.Gatekeeper{
			ObjectMeta: metav1.ObjectMeta{
				UID: "test-1",
			},
			Spec: operatorv1alpha1.GatekeeperSpec{
				Config: &operatorv1alpha1.ConfigConfig{
					Matches: []v1alpha1.MatchEntry{{
						ExcludedNamespaces: []wildcard.Wildcard{
							"cat-ns", "dog-ns",
						},
						Processes: []string{
							"audit", "webhook", "sync",
						},
					}},
				},
			},
		}

		_ = reconciler.setExemptNamespaces(ctx, config, gatekeeper)
		g.Expect(config.Spec.Match).Should(HaveLen(2))
		g.Expect(config.Spec.Match[0]).Should(BeComparableTo(defaultConfig.Spec.Match[0]))
		g.Expect(config.Spec.Match[1]).Should(BeComparableTo(gatekeeper.Spec.Config.Matches[0]))
	})

	t.Run("Should be same result when execute setExemptNamespaces several times ", func(t *testing.T) {
		config = &v1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "config",
			},
			Spec: v1alpha1.ConfigSpec{},
		}

		disableDefaultMatches = false

		gatekeeper = &operatorv1alpha1.Gatekeeper{
			ObjectMeta: metav1.ObjectMeta{
				UID: "test-1",
			},
			Spec: operatorv1alpha1.GatekeeperSpec{
				Config: &operatorv1alpha1.ConfigConfig{
					Matches: []v1alpha1.MatchEntry{{
						ExcludedNamespaces: []wildcard.Wildcard{
							"cat-ns", "dog-ns",
						},
						Processes: []string{
							"audit", "webhook", "sync",
						},
					}},
					DisableDefaultMatches: disableDefaultMatches,
				},
			},
		}
		// First try
		_ = reconciler.setExemptNamespaces(ctx, config, gatekeeper)

		// Second try
		_ = reconciler.setExemptNamespaces(ctx, config, gatekeeper)

		// Third try
		_ = reconciler.setExemptNamespaces(ctx, config, gatekeeper)
		g.Expect(config.Spec.Match).Should(HaveLen(2))
		g.Expect(config.Spec.Match[0]).Should(BeComparableTo(defaultConfig.Spec.Match[0]))
		g.Expect(config.Spec.Match[1]).Should(BeComparableTo(gatekeeper.Spec.Config.Matches[0]))
	})

	t.Run("Should not change config when ownerRef is different and match exist", func(t *testing.T) {
		config = &v1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "config",
			},
			Spec: v1alpha1.ConfigSpec{
				Match: []v1alpha1.MatchEntry{{
					ExcludedNamespaces: []wildcard.Wildcard{
						"cat-ns", "dog-ns",
					},
					Processes: []string{
						"audit", "webhook", "sync",
					},
				}},
			},
		}

		disableDefaultMatches = false

		gatekeeper = &operatorv1alpha1.Gatekeeper{
			ObjectMeta: metav1.ObjectMeta{
				UID: "test-22",
			},
			Spec: operatorv1alpha1.GatekeeperSpec{
				Config: &operatorv1alpha1.ConfigConfig{
					Matches: []v1alpha1.MatchEntry{{
						ExcludedNamespaces: []wildcard.Wildcard{
							"tiger-ns", "rabbit-ns",
						},
						Processes: []string{
							"audit",
						},
					}},
				},
			},
		}

		_ = reconciler.setExemptNamespaces(ctx, config, gatekeeper)

		g.Expect(config.Spec.Match).Should(HaveLen(1))
		g.Expect(config.Spec.Match[0]).Should(BeComparableTo(config.Spec.Match[0]))
	})
}
