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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	helixclient "github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	appv1alpha1 "github.com/helixml/helix/operator/api/v1alpha1"
)

const projectFinalizerName = "project.aispec.org/finalizer"

// ProjectReconciler reconciles a Project object.
type ProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	helix  *helixclient.HelixClient
	orgID  string // Helix organization ID (from HELIX_ORGANIZATION_ID env var)
}

// +kubebuilder:rbac:groups=app.aispec.org,resources=projects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=app.aispec.org,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=app.aispec.org,resources=projects/finalizers,verbs=update

// Reconcile handles the reconciliation loop for Project resources.
func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var project appv1alpha1.Project
	if err := r.Get(ctx, req.NamespacedName, &project); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling Project", "name", req.NamespacedName)

	// Build namespaced project name
	projectName := fmt.Sprintf("%s%s%s%s%s", k8sPrefix, k8sSeparator, req.Namespace, k8sSeparator, project.Name)

	// Handle deletion
	if !project.DeletionTimestamp.IsZero() {
		return r.handleProjectDeletion(ctx, &project)
	}

	// Add finalizer if missing
	if !containsString(project.Finalizers, projectFinalizerName) {
		project.Finalizers = append(project.Finalizers, projectFinalizerName)
		if err := r.Update(ctx, &project); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Build apply request
	applyReq := &types.ProjectApplyRequest{
		OrganizationID: r.orgID,
		Name:           projectName,
		Spec: types.ProjectSpec{
			Description: project.Spec.Description,
			Guidelines:  project.Spec.Guidelines,
		},
	}

	// Map repository specs
	if project.Spec.Repository != nil {
		applyReq.Spec.Repository = &types.ProjectRepositorySpec{
			URL:           project.Spec.Repository.URL,
			DefaultBranch: project.Spec.Repository.DefaultBranch,
			Primary:       project.Spec.Repository.Primary,
		}
	}
	for _, r2 := range project.Spec.Repositories {
		applyReq.Spec.Repositories = append(applyReq.Spec.Repositories, types.ProjectRepositorySpec{
			URL:           r2.URL,
			DefaultBranch: r2.DefaultBranch,
			Primary:       r2.Primary,
		})
	}

	// Map startup
	if project.Spec.Startup != nil {
		applyReq.Spec.Startup = &types.ProjectStartup{
			Script:  project.Spec.Startup.Script,
			Install: project.Spec.Startup.Install,
			Start:   project.Spec.Startup.Start,
		}
	}
	applyReq.Spec.AutoStartBacklogTasks = project.Spec.AutoStartBacklogTasks

	// Map kanban
	if project.Spec.Kanban != nil {
		applyReq.Spec.Kanban = &types.ProjectKanban{}
		if project.Spec.Kanban.WIPLimits != nil {
			applyReq.Spec.Kanban.WIPLimits = &types.ProjectWIPLimits{
				Planning:       project.Spec.Kanban.WIPLimits.Planning,
				Implementation: project.Spec.Kanban.WIPLimits.Implementation,
				Review:         project.Spec.Kanban.WIPLimits.Review,
			}
		}
	}

	// Map tasks
	for _, t := range project.Spec.Tasks {
		applyReq.Spec.Tasks = append(applyReq.Spec.Tasks, types.ProjectTaskSpec{
			Title:       t.Title,
			Description: t.Description,
		})
	}

	// Map agent spec
	if project.Spec.Agent != nil {
		a := project.Spec.Agent
		agentSpec := &types.ProjectAgentSpec{
			Name:        a.Name,
			Runtime:     a.Runtime,
			Model:       a.Model,
			Provider:    a.Provider,
			Credentials: a.Credentials,
		}
		if a.Tools != nil {
			agentSpec.Tools = &types.ProjectAgentTools{
				WebSearch:  a.Tools.WebSearch,
				Browser:    a.Tools.Browser,
				Calculator: a.Tools.Calculator,
			}
		}
		if a.Display != nil {
			agentSpec.Display = &types.ProjectAgentDisplay{
				Resolution:  a.Display.Resolution,
				DesktopType: a.Display.DesktopType,
				FPS:         a.Display.FPS,
			}
		}
		applyReq.Spec.Agent = agentSpec
	}

	resp, err := r.helix.ApplyProject(ctx, applyReq)
	if err != nil {
		logger.Error(err, "Failed to apply project", "name", projectName)
		now := metav1.Now()
		project.Status.Ready = false
		project.Status.Message = err.Error()
		project.Status.LastSynced = &now
		_ = r.Status().Update(ctx, &project)
		return ctrl.Result{}, err
	}

	now := metav1.Now()
	project.Status.Ready = true
	project.Status.ProjectID = resp.ProjectID
	project.Status.AgentAppID = resp.AgentAppID
	project.Status.LastSynced = &now
	project.Status.Message = ""

	if err := r.Status().Update(ctx, &project); err != nil {
		logger.Error(err, "Failed to update project status")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully reconciled project", "name", projectName, "id", resp.ProjectID)
	return ctrl.Result{}, nil
}

func (r *ProjectReconciler) handleProjectDeletion(ctx context.Context, project *appv1alpha1.Project) (ctrl.Result, error) {
	if containsString(project.Finalizers, projectFinalizerName) {
		project.Finalizers = removeString(project.Finalizers, projectFinalizerName)
		if err := r.Update(ctx, project); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	helixURL := os.Getenv("HELIX_URL")
	if helixURL == "" {
		return fmt.Errorf("HELIX_URL environment variable is required")
	}
	helixAPIKey := os.Getenv("HELIX_API_KEY")
	if helixAPIKey == "" {
		return fmt.Errorf("HELIX_API_KEY environment variable is required")
	}

	tlsSkipVerify := os.Getenv("HELIX_TLS_SKIP_VERIFY") == "true"

	var err error
	r.helix, err = helixclient.NewClient(helixURL, helixAPIKey, tlsSkipVerify)
	if err != nil {
		return fmt.Errorf("failed to create Helix client: %w", err)
	}

	r.orgID = os.Getenv("HELIX_ORGANIZATION_ID")

	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1alpha1.Project{}).
		Named("project").
		Complete(r)
}
