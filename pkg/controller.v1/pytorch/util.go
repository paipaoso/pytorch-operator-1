// Copyright 2018 The Kubeflow Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pytorch

import (
	"fmt"
	"bytes"
	"time"
	"html/template"

	pyv1 "github.com/kubeflow/pytorch-operator/pkg/apis/pytorch/v1"
		pylogger "github.com/kubeflow/tf-operator/pkg/logger"
		metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
		common "github.com/kubeflow/common/job_controller/api/v1"
	"github.com/kubeflow/pytorch-operator/pkg/common/config"
	"github.com/kubernetes-sigs/yaml"
	v1 "k8s.io/api/core/v1"
)

var (
	errPortNotFound = fmt.Errorf("failed to found the port")
)


// GetPortFromPyTorchJob gets the port of pytorch container.
func GetPortFromPyTorchJob(job *pyv1.PyTorchJob, rtype pyv1.PyTorchReplicaType) (int32, error) {
	containers := job.Spec.PyTorchReplicaSpecs[rtype].Template.Spec.Containers
	for _, container := range containers {
		if container.Name == pyv1.DefaultContainerName {
			ports := container.Ports
			for _, port := range ports {
				if port.Name == pyv1.DefaultPortName {
					return port.ContainerPort, nil
				}
			}
		}
	}
	return -1, errPortNotFound
}

type InitContainerParam struct {
	MasterAddr         string
	InitContainerImage string
}

func ContainMasterSpec(job *pyv1.PyTorchJob) bool {
	if _, ok := job.Spec.PyTorchReplicaSpecs[pyv1.PyTorchReplicaTypeMaster]; ok {
		return true
	}
	return false
}

func GetInitContainer(containerTemplate string, param InitContainerParam) ([]v1.Container, error) {
	var buf bytes.Buffer
	tpl, err := template.New("container").Parse(containerTemplate)
	if err != nil {
		return nil, err
	}
	if err := tpl.Execute(&buf, param); err != nil {
		return nil, err
	}

	var result []v1.Container
	err = yaml.Unmarshal(buf.Bytes(), &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func AddInitContainerForWorkerPod(podTemplate *v1.PodTemplateSpec, param InitContainerParam) error {
	containers, err := GetInitContainer(config.GetInitContainerTemplate(), param)
	if err != nil {
		return err
	}
	podTemplate.Spec.InitContainers = append(podTemplate.Spec.InitContainers, containers...)
	return nil
}

// jobSuspended returns whether a Job is suspended while taking the feature
// gate into account.
func jobSuspended(job *pyv1.PyTorchJob) bool {
	return job.Spec.Suspend != nil && *job.Spec.Suspend
}

func needReconcile(job *pyv1.PyTorchJob) bool {
	return !(job.Status.CompletionTime != nil && (isSucceeded(job.Status) || isFailed(job.Status)))

}

func SetSucceed(job *pyv1.PyTorchJob) error {
	currentTime := metav1.Now()
	gracefultime, _ := time.ParseDuration("30m")

	if isPartialSucceed(job.Status) && currentTime.After(job.Status.PartialSucceedTime.Time.Add(gracefultime)) {
	msg := fmt.Sprintf("PyTorchJob %s has succeed.", job.Name)
		err := updatePyTorchJobConditions(job, common.JobSucceeded, pytorchJobSucceededReason, msg)
		if err != nil {
			pylogger.LoggerForJob(job).Infof("Append job condition error: %v", err)
			return err
		}
		pytorchJobsSuccessCount.Inc()
	}
	return nil
}
