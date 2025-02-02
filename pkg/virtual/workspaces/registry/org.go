/*
Copyright 2022 The KCP Authors.

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

package registry

import (
	"time"

	rbacinformers "k8s.io/client-go/informers/rbac/v1"
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"

	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
	tenancyclient "github.com/kcp-dev/kcp/pkg/client/clientset/versioned/typed/tenancy/v1alpha1"
	workspaceinformer "github.com/kcp-dev/kcp/pkg/client/informers/externalversions/tenancy/v1alpha1"
	frameworkrbac "github.com/kcp-dev/kcp/pkg/virtual/framework/rbac"
	workspaceauth "github.com/kcp-dev/kcp/pkg/virtual/workspaces/authorization"
)

// CreateAndStartOrg creates an Org that contains all the required clients and caches to retrieve user workspaces inside an org
// As part of an Org, a WorkspaceAuthCache is created and ensured to be started.
func CreateAndStartOrg(
	orgRBACClient rbacv1client.RbacV1Interface,
	orgClusteWorkspaceClient tenancyclient.ClusterWorkspaceInterface,
	orgRBACInformers rbacinformers.Interface,
	orgCRBInformer rbacinformers.ClusterRoleBindingInformer,
	orgClusterWorkspaceInformer workspaceinformer.ClusterWorkspaceInformer,
) *Org {
	orgSubjectLocator := frameworkrbac.NewSubjectLocator(orgRBACInformers)
	orgReviewer := workspaceauth.NewReviewer(orgSubjectLocator)

	orgWorkspaceAuthorizationCache := workspaceauth.NewAuthorizationCache(
		orgClusterWorkspaceInformer.Lister(),
		orgClusterWorkspaceInformer.Informer(),
		orgReviewer,
		*workspaceauth.NewAttributesBuilder().
			Verb("get").
			Resource(tenancyv1alpha1.SchemeGroupVersion.WithResource("clusterworkspaces"), "workspace").
			AttributesRecord,
		orgRBACInformers,
	)

	newOrg := &Org{
		rbacClient:             orgRBACClient,
		crbInformer:            orgCRBInformer,
		crbLister:              orgCRBInformer.Lister(),
		workspaceReviewer:      orgReviewer,
		clusterWorkspaceClient: orgClusteWorkspaceClient,
		clusterWorkspaceLister: orgWorkspaceAuthorizationCache,
		stopCh:                 make(chan struct{}),
		authCache:              orgWorkspaceAuthorizationCache,
	}

	newOrg.authCache.Run(1*time.Second, newOrg.stopCh)

	return newOrg
}

func NewRootOrg(
	rootRBACClient rbacv1client.RbacV1Interface,
	rootCRBInformer rbacinformers.ClusterRoleBindingInformer,
	rootReviewer *workspaceauth.Reviewer,
	rootClusteWorkspaceClient tenancyclient.ClusterWorkspaceInterface,
	rootWorkspaceAuthorizationCache *workspaceauth.AuthorizationCache,
) *Org {
	return &Org{
		rbacClient:             rootRBACClient,
		crbInformer:            rootCRBInformer,
		workspaceReviewer:      rootReviewer,
		clusterWorkspaceClient: rootClusteWorkspaceClient,
		clusterWorkspaceLister: rootWorkspaceAuthorizationCache,
		authCache:              rootWorkspaceAuthorizationCache,
	}
}

type Org struct {
	rbacClient             rbacv1client.RbacV1Interface
	crbInformer            rbacinformers.ClusterRoleBindingInformer
	crbLister              rbacv1listers.ClusterRoleBindingLister
	clusterWorkspaceClient tenancyclient.ClusterWorkspaceInterface

	// workspaceReviewer checks permissions for a given verb to workspaces
	workspaceReviewer *workspaceauth.Reviewer
	// workspaceLister can enumerate workspace lists that enforce policy
	clusterWorkspaceLister workspaceauth.Lister
	// authCache is a cache of cluster workspaces and associated subjects for a given org.
	authCache *workspaceauth.AuthorizationCache
	// stopCh allows stopping the authCache for this org.
	stopCh chan struct{}
}

func (o Org) Ready() bool {
	return o.authCache.ReadyForAccess()
}

func (o Org) Stop() {
	o.stopCh <- struct{}{}
}
