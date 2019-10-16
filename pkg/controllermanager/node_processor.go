/*
Copyright 2019 Baidu, Inc.

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

package controllermanager

import (
	"encoding/json"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/baidu/ote-stack/pkg/reporter"
)

func (u *UpstreamProcessor) handleNodeReport(b []byte) error {
	// Deserialize byte data to NodeReportStatus
	nrs, err := NodeReportStatusDeserialize(b)
	if err != nil {
		return fmt.Errorf("NodeReportStatusDeserialize failed : %v", err)
	}
	// handle FullList
	if nrs.FullList != nil {
		// TODO:handle full node resource.
	}
	// handle UpdateMap
	if nrs.UpdateMap != nil {
		u.handleNodeUpdateMap(nrs.UpdateMap)
	}
	// handle DelMap
	if nrs.DelMap != nil {
		u.handleNodeDelMap(nrs.DelMap)
	}

	return nil
}

func (u *UpstreamProcessor) handleNodeDelMap(delMap map[string]*corev1.Node) {
	for _, node := range delMap {
		err := UniqueResourceName(&node.ObjectMeta)
		if err != nil {
			klog.Errorf("handleNodeDelMap's UniqueResourceName method failed, %s", err)
			continue
		}

		err = u.DeleteNode(node)
		if err != nil {
			klog.Errorf("Report node delete event failed : %v", err)
			continue
		}

		klog.V(3).Infof("Report node delete event success: name(%s)", node.Name)
	}
}

func (u *UpstreamProcessor) handleNodeUpdateMap(updateMap map[string]*corev1.Node) {
	for _, node := range updateMap {
		err := UniqueResourceName(&node.ObjectMeta)
		if err != nil {
			klog.Errorf("handleNodeUpdateMap's UniqueResourceName method failed, %s", err)
			continue
		}

		err = u.CreateOrUpdateNode(node)
		if err != nil {
			klog.Errorf("Report node create or update event failed : %v", err)
			continue
		}
	}
}

// NodeReportStatusDeserialize deserialize byte data to NodeReportStatus.
func NodeReportStatusDeserialize(b []byte) (*reporter.NodeResourceStatus, error) {
	nodeReportStatus := reporter.NodeResourceStatus{}
	err := json.Unmarshal(b, &nodeReportStatus)
	if err != nil {
		return nil, err
	}
	return &nodeReportStatus, nil
}

// GetNode will retrieve the requested node based on name.
func (u *UpstreamProcessor) GetNode(node *corev1.Node) (*corev1.Node, error) {
	storedNode, err := u.ctx.K8sClient.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return storedNode, err
}

// CreateNode will create the given node.
func (u *UpstreamProcessor) CreateNode(node *corev1.Node) error {
	_, err := u.ctx.K8sClient.CoreV1().Nodes().Create(node)
	if err != nil {
		return err
	}

	klog.V(3).Infof("Report node create event success: name(%s)", node.Name)

	return nil
}

// UpdateNode will update the given node.
func (u *UpstreamProcessor) UpdateNode(node *corev1.Node) error {
	storedNode, err := u.GetNode(node)
	if err != nil {
		return err
	}

	err = u.checkNodeEdgeVersion(node, storedNode)
	if err != nil {
		return err
	}

	node.ResourceVersion = storedNode.ResourceVersion
	_, err = u.ctx.K8sClient.CoreV1().Nodes().Update(node)

	// In the case of concurrency, try again if a conflict occurs
	if err != nil && errors.IsConflict(err) {
		return u.UpdateNode(node)
	}

	if err != nil {
		return err
	}

	klog.V(3).Infof("Report node update event success: name(%s)", node.Name)

	return nil
}

// CreateOrUpdateNode will update the given node or create it if does not exist.
func (u *UpstreamProcessor) CreateOrUpdateNode(node *corev1.Node) error {
	_, err := u.GetNode(node)
	// If not found resource, create it.
	if err != nil && errors.IsNotFound(err) {
		return u.CreateNode(node)
	}

	if err != nil {
		return err
	}

	return u.UpdateNode(node)
}

func (u *UpstreamProcessor) checkNodeEdgeVersion(node *corev1.Node, storedNode *corev1.Node) error {
	if node.Labels[reporter.EdgeVersionLabel] == "" || storedNode.Labels[reporter.EdgeVersionLabel] == "" {
		return fmt.Errorf("node edge-version is empty")
	}
	// resource report sequential checking
	nodeVersion, err := strconv.Atoi(node.Labels[reporter.EdgeVersionLabel])
	if err != nil {
		return err
	}

	storedNodeVersion, err := strconv.Atoi(storedNode.Labels[reporter.EdgeVersionLabel])
	if err != nil {
		return err
	}

	// resource report sequential checking
	if nodeVersion <= storedNodeVersion {
		return fmt.Errorf("Current node edge-version(%s) is less than or equal to ETCD's node edge-version(%s)",
			node.Labels[reporter.EdgeVersionLabel], storedNode.Labels[reporter.EdgeVersionLabel])
	}

	return nil
}

// DeleteNode will delete the given node.
func (u *UpstreamProcessor) DeleteNode(node *corev1.Node) error {
	return u.ctx.K8sClient.CoreV1().Nodes().Delete(node.Name, &metav1.DeleteOptions{})
}
