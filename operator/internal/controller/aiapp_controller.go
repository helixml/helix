/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,

WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	helixclient "github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	appv1 "github.com/helixml/helix/operator/api/v1"
)

const (
	// Prefix for k8s managed apps, using . as separator since it's URL-safe
	k8sPrefix    = "k8s"
	k8sSeparator = "."
)

// AIAppReconciler reconciles a AIApp object
type AIAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	helix  *helixclient.HelixClient
}

// +kubebuilder:rbac:groups=app.aispec.org,resources=aiapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=app.aispec.org,resources=aiapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=app.aispec.org,resources=aiapps/finalizers,verbs=update

// Reconcile handles the reconciliation loop for AIApp resources
func (r *AIAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the AIApp resource
	var aiapp appv1.AIApp
	if err := r.Get(ctx, req.NamespacedName, &aiapp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling AIApp", "name", req.NamespacedName)

	// Create namespaced app ID to prevent clashes and identify k8s-managed apps
	// Using dots instead of slashes for URL safety
	appID := fmt.Sprintf("%s%s%s%s%s", k8sPrefix, k8sSeparator, req.Namespace, k8sSeparator, aiapp.Name)

	// Convert CRD to Helix App type
	app := &types.App{
		ID: appID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        aiapp.Spec.Name,
				Description: aiapp.Spec.Description,
				Avatar:      aiapp.Spec.Avatar,
				Image:       aiapp.Spec.Image,
				Assistants:  make([]types.AssistantConfig, 0, len(aiapp.Spec.Assistants)),
			},
		},
	}

	// Convert assistants
	for _, assistant := range aiapp.Spec.Assistants {
		helixAssistant := types.AssistantConfig{
			ID:                   assistant.ID,
			Name:                 assistant.Name,
			Description:          assistant.Description,
			Avatar:               assistant.Avatar,
			Image:                assistant.Image,
			Provider:             types.Provider(assistant.Provider),
			Model:                assistant.Model,
			Type:                 types.SessionType(assistant.Type),
			SystemPrompt:         assistant.SystemPrompt,
			RAGSourceID:          assistant.RAGSourceID,
			LoraID:               assistant.LoraID,
			IsActionableTemplate: assistant.IsActionableTemplate,
		}

		// Convert APIs
		for _, api := range assistant.APIs {
			helixAssistant.APIs = append(helixAssistant.APIs, types.AssistantAPI{
				Name:                    api.Name,
				Description:             api.Description,
				Schema:                  api.Schema,
				URL:                     api.URL,
				Headers:                 api.Headers,
				Query:                   api.Query,
				RequestPrepTemplate:     api.RequestPrepTemplate,
				ResponseSuccessTemplate: api.ResponseSuccessTemplate,
				ResponseErrorTemplate:   api.ResponseErrorTemplate,
			})
		}

		// Convert GPTScripts
		for _, script := range assistant.GPTScripts {
			helixAssistant.GPTScripts = append(helixAssistant.GPTScripts, types.AssistantGPTScript{
				Name:        script.Name,
				Description: script.Description,
				File:        script.File,
				Content:     script.Content,
			})
		}

		// Convert Zapier configs
		for _, zapier := range assistant.Zapier {
			helixAssistant.Zapier = append(helixAssistant.Zapier, types.AssistantZapier{
				Name:          zapier.Name,
				Description:   zapier.Description,
				APIKey:        zapier.APIKey,
				Model:         zapier.Model,
				MaxIterations: zapier.MaxIterations,
			})
		}

		app.Config.Helix.Assistants = append(app.Config.Helix.Assistants, helixAssistant)
	}

	// Check if app exists in Helix API
	_, err := r.helix.GetApp(app.ID)
	if err != nil {
		// If app doesn't exist, create it
		if err.Error() == "not found" {
			logger.Info("Creating new app in Helix", "id", app.ID)
			_, err = r.helix.CreateApp(app)
			if err != nil {
				logger.Error(err, "Failed to create app in Helix", "id", app.ID)
				return ctrl.Result{}, fmt.Errorf("failed to create app in Helix: %w", err)
			}
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get app from Helix", "id", app.ID)
		return ctrl.Result{}, fmt.Errorf("failed to get app from Helix: %w", err)
	}

	// Update existing app - we know it's k8s managed because of the k8s prefix
	logger.Info("Updating existing app in Helix", "id", app.ID)
	_, err = r.helix.UpdateApp(app)
	if err != nil {
		logger.Error(err, "Failed to update app in Helix", "id", app.ID)
		return ctrl.Result{}, fmt.Errorf("failed to update app in Helix: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AIAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize Helix client
	helixURL := os.Getenv("HELIX_URL")
	if helixURL == "" {
		return fmt.Errorf("HELIX_URL environment variable is required")
	}

	helixAPIKey := os.Getenv("HELIX_API_KEY")
	if helixAPIKey == "" {
		return fmt.Errorf("HELIX_API_KEY environment variable is required")
	}

	logger := log.FromContext(context.Background())
	logger.Info("Initializing Helix client", "url", helixURL)

	var err error
	r.helix, err = helixclient.NewClient(helixURL, helixAPIKey)
	if err != nil {
		return fmt.Errorf("failed to create Helix client: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1.AIApp{}).
		Named("aiapp").
		Complete(r)
}
