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
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	helixclient "github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	appv1alpha1 "github.com/helixml/helix/operator/api/v1alpha1"
)

const (
	// Prefix for k8s managed apps, using . as separator since it's URL-safe
	k8sPrefix     = "k8s"
	k8sSeparator  = "."
	finalizerName = "app.aispec.org/finalizer"
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
	var aiapp appv1alpha1.AIApp
	if err := r.Get(ctx, req.NamespacedName, &aiapp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling AIApp", "name", req.NamespacedName)

	// Create namespaced app ID to prevent clashes and identify k8s-managed apps
	// Using dots instead of slashes for URL safety
	appID := fmt.Sprintf("%s%s%s%s%s", k8sPrefix, k8sSeparator, req.Namespace, k8sSeparator, aiapp.Name)

	// Handle deletion
	if !aiapp.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &aiapp, appID)
	}

	// Add finalizer if it doesn't exist
	if !containsString(aiapp.Finalizers, finalizerName) {
		aiapp.Finalizers = append(aiapp.Finalizers, finalizerName)
		if err := r.Update(ctx, &aiapp); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Convert CRD to Helix App type
	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        appID,
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
			Provider:             assistant.Provider,
			Model:                assistant.Model,
			SystemPrompt:         assistant.SystemPrompt,
			RAGSourceID:          assistant.RAGSourceID,
			LoraID:               assistant.LoraID,
			IsActionableTemplate: assistant.IsActionableTemplate,
		}

		// Convert APIs
		for _, api := range assistant.APIs {
			logger.Info("Converting API", "name", api.Name)
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
	logger.Info("Checking if app exists in Helix", "name", appID)
	existingApp, err := r.helix.GetAppByName(ctx, appID)
	if err != nil {
		errStr := strings.ToLower(err.Error())
		logger.Info("Error checking app existence", "error", errStr)
		// Check for both 404 status code and "not found" message
		if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
			logger.Info("App not found in Helix, creating new app", "name", appID)
			createdApp, err := r.helix.CreateApp(ctx, app)
			if err != nil {
				logger.Error(err, "Failed to create app in Helix", "name", appID, "error", err.Error())
				return ctrl.Result{}, fmt.Errorf("failed to create app in Helix: %w", err)
			}
			logger.Info("Successfully created new app", "name", appID, "id", createdApp.ID)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get app from Helix", "name", appID, "error", err.Error())
		return ctrl.Result{}, fmt.Errorf("failed to get app from Helix: %w", err)
	}
	logger.Info("Successfully retrieved app from Helix", "name", appID, "id", existingApp.ID)

	// Update existing app
	logger.Info("Preparing to update existing app in Helix", "name", appID, "id", existingApp.ID,
		"existing_owner", existingApp.Owner,
		"existing_name", existingApp.Config.Helix.Name,
		"new_name", app.Config.Helix.Name)

	// Preserve existing metadata
	app.ID = existingApp.ID
	app.Owner = existingApp.Owner
	app.Created = existingApp.Created
	app.Updated = existingApp.Updated
	app.OwnerType = existingApp.OwnerType

	logger.Info("Sending update request to Helix", "name", appID, "id", existingApp.ID,
		"update_payload", fmt.Sprintf("%+v", app))

	updatedApp, err := r.helix.UpdateApp(ctx, app)
	if err != nil {
		logger.Error(err, "Failed to update app in Helix", "name", appID, "id", app.ID)
		return ctrl.Result{}, fmt.Errorf("failed to update app %s in Helix: %w", app.ID, err)
	}
	logger.Info("Successfully updated existing app", "name", appID, "id", updatedApp.ID)
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
	// TODO: Add HELIX_TLS_SKIP_VERIFY env var support for enterprise deployments
	r.helix, err = helixclient.NewClient(helixURL, helixAPIKey, false)
	if err != nil {
		return fmt.Errorf("failed to create Helix client: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1alpha1.AIApp{}).
		Named("aiapp").
		Complete(r)
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

func (r *AIAppReconciler) handleDeletion(ctx context.Context, aiapp *appv1alpha1.AIApp, appID string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if containsString(aiapp.Finalizers, finalizerName) {
		// Delete the app from Helix
		existingApp, err := r.helix.GetAppByName(ctx, appID)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			// If the app doesn't exist in Helix, we can proceed with removing the finalizer
			if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
				logger.Info("App already deleted from Helix or doesn't exist", "appID", appID)
			} else {
				logger.Error(err, "Failed to get app from Helix during deletion", "appID", appID)
				return ctrl.Result{}, err
			}
		} else {
			// Delete the app from Helix
			logger.Info("Deleting app from Helix", "appID", existingApp.ID)
			if err := r.helix.DeleteApp(ctx, existingApp.ID, true); err != nil {
				logger.Error(err, "Failed to delete app from Helix", "appID", existingApp.ID)
				return ctrl.Result{}, err
			}
			logger.Info("Successfully deleted app from Helix", "appID", existingApp.ID)
		}

		// Remove the finalizer
		aiapp.Finalizers = removeString(aiapp.Finalizers, finalizerName)
		if err := r.Update(ctx, aiapp); err != nil {
			logger.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
		logger.Info("Successfully removed finalizer", "appID", appID)
	}

	return ctrl.Result{}, nil
}
