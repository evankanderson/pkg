/*
Copyright 2019 The Knative Authors

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

package metrics

import (
	"contrib.go.opencensus.io/exporter/stackdriver/monitoredresource"
	"go.opencensus.io/metric/metricdata"
	"go.opencensus.io/resource"
	"knative.dev/pkg/metrics/metricskey"
)

// TODO should be moved to serving. See https://github.com/knative/pkg/issues/608

type KnativeRevision struct {
	Project           string
	Location          string
	ClusterName       string
	NamespaceName     string
	ServiceName       string
	ConfigurationName string
	RevisionName      string
}

func (kr *KnativeRevision) MonitoredResource() (resType string, labels map[string]string) {
	labels = map[string]string{
		metricskey.LabelProject:           kr.Project,
		metricskey.LabelLocation:          kr.Location,
		metricskey.LabelClusterName:       kr.ClusterName,
		metricskey.LabelNamespaceName:     kr.NamespaceName,
		metricskey.LabelServiceName:       kr.ServiceName,
		metricskey.LabelConfigurationName: kr.ConfigurationName,
		metricskey.LabelRevisionName:      kr.RevisionName,
	}
	return metricskey.ResourceTypeKnativeRevision, labels
}

func GetKnativeRevisionMonitoredResource(
	des *metricdata.Descriptor, tags map[string]string, gm *gcpMetadata, r *resource.Resource) (map[string]string, monitoredresource.Interface) {
	kr := &KnativeRevision{
		// The first three resource labels are from metadata.
		Project:     gm.project,
		Location:    gm.location,
		ClusterName: gm.cluster,
		// The rest resource labels are from metrics labels.
		NamespaceName:     metricskey.ValueUnknown,
		ServiceName:       metricskey.ValueUnknown,
		ConfigurationName: metricskey.ValueUnknown,
		RevisionName:      metricskey.ValueUnknown,
	}

	metricLabels := make(map[string]string, len(tags))
	for k, v := range tags {
		if !setKnativeRevisionField(kr, k, v) {
			metricLabels[k] = v
		}
	}

	if r != nil {
		for k, v := range r.Labels {
			setKnativeRevisionField(kr, k, v)
		}
	}

	return metricLabels, kr
}

func setKnativeRevisionField(kr *KnativeRevision, k string, v string) bool {
	switch k {
	case metricskey.LabelNamespaceName:
		kr.NamespaceName = v
	case metricskey.LabelServiceName:
		kr.ServiceName = v
	case metricskey.LabelConfigurationName:
		kr.ConfigurationName = v
	case metricskey.LabelRevisionName:
		kr.RevisionName = v
	default:
		return false
	}
	return true
}
