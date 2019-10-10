/*
Copyright 2017 The Kubernetes Authors.

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

package server

import (
	"os"
	"path/filepath"
	"text/template"

	"github.com/containerd/containerd/log"
	cni "github.com/containerd/go-cni"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// cniConfigTemplate contains the values containerd will overwrite
// in the cni config template.
type cniConfigTemplate struct {
	// PodCIDR is the cidr for pods on the node.
	PodCIDR string
}

// cniConfigFileName is the name of cni config file generated by containerd.
const cniConfigFileName = "10-containerd-net.conflist"

// UpdateRuntimeConfig updates the runtime config. Currently only handles podCIDR updates.
func (c *criService) UpdateRuntimeConfig(ctx context.Context, r *runtime.UpdateRuntimeConfigRequest) (*runtime.UpdateRuntimeConfigResponse, error) {
	podCIDR := r.GetRuntimeConfig().GetNetworkConfig().GetPodCidr()
	if podCIDR == "" {
		return &runtime.UpdateRuntimeConfigResponse{}, nil
	}
	confTemplate := c.config.NetworkPluginConfTemplate
	if confTemplate == "" {
		log.G(ctx).Info("No cni config template is specified, wait for other system components to drop the config.")
		return &runtime.UpdateRuntimeConfigResponse{}, nil
	}
	if err := c.netPlugin.Status(); err == nil {
		log.G(ctx).Infof("Network plugin is ready, skip generating cni config from template %q", confTemplate)
		return &runtime.UpdateRuntimeConfigResponse{}, nil
	} else if err := c.netPlugin.Load(cni.WithLoNetwork, cni.WithDefaultConf); err == nil {
		log.G(ctx).Infof("CNI config is successfully loaded, skip generating cni config from template %q", confTemplate)
		return &runtime.UpdateRuntimeConfigResponse{}, nil
	}
	log.G(ctx).Infof("Generating cni config from template %q", confTemplate)
	// generate cni config file from the template with updated pod cidr.
	t, err := template.ParseFiles(confTemplate)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse cni config template %q", confTemplate)
	}
	if err := os.MkdirAll(c.config.NetworkPluginConfDir, 0755); err != nil {
		return nil, errors.Wrapf(err, "failed to create cni config directory: %q", c.config.NetworkPluginConfDir)
	}
	confFile := filepath.Join(c.config.NetworkPluginConfDir, cniConfigFileName)
	f, err := os.OpenFile(confFile, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open cni config file %q", confFile)
	}
	defer f.Close()
	if err := t.Execute(f, cniConfigTemplate{PodCIDR: podCIDR}); err != nil {
		return nil, errors.Wrapf(err, "failed to generate cni config file %q", confFile)
	}
	return &runtime.UpdateRuntimeConfigResponse{}, nil
}