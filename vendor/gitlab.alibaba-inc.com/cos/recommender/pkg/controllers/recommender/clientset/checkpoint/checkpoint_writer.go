package checkpoint

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/model"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/util"
	recv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	rec_types "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned/typed/autoscaling/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

// CheckpointWriter persistently stores aggregated historical usage of containers
// controlled by Recommendation objects. This state can be restored to initialize the model after restart.
type CheckpointWriter interface {
	// StoreCheckpoints writes
	StoreCheckpoints(ctx context.Context, now time.Time, minCheckpoints int) error
}

type checkpointWriter struct {
	recommendationCheckpointClient rec_types.RecommendationCheckpointsGetter
	cluster                        *model.ClusterState
}

func NewCheckpointWriter(cluster *model.ClusterState, recCheckpointClient rec_types.RecommendationCheckpointsGetter) CheckpointWriter {
	return &checkpointWriter{
		recommendationCheckpointClient: recCheckpointClient,
		cluster:                        cluster,
	}
}

func isFetchingHistory(recommendation *model.Recommendation) bool {
	condition, found := recommendation.Conditions[recv1alpha1.FetchingHistory]
	if !found {
		return false
	}
	return condition.Status == v1.ConditionTrue
}

func getRecommendationsToCheckpoint(clusterRecommendations map[model.RecommendationID]*model.Recommendation) []*model.Recommendation {
	recommendations := make([]*model.Recommendation, 0, len(clusterRecommendations))
	for _, rec := range clusterRecommendations {
		if isFetchingHistory(rec) {
			klog.V(3).Infof("Recommendation %s/%s is loading history, skipping checkpoints", rec.ID.Namespace, rec.ID.RecommendationName)
			continue
		}
		recommendations = append(recommendations, rec)
	}
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].CheckpointWritten.Before(recommendations[j].CheckpointWritten)
	})
	return recommendations
}

func buildAggregateContainerStateMap(recommendation *model.Recommendation, cluster *model.ClusterState, now time.Time) map[string]*model.AggregateContainerState {
	aggregateContainerStateMap := recommendation.AggregateStateByContainerName()

	for _, pod := range cluster.Pods {
		for containerName, container := range pod.Containers {
			aggregateKey := cluster.MakeAggregateStateKey(pod, containerName)
			if recommendation.UsesAggregation(aggregateKey) {
				if aggregateContainerState, exists := aggregateContainerStateMap[containerName]; exists {
					subtractCurrentContainerMemoryPeak(aggregateContainerState, container, now)
				}
			}
		}
	}
	return aggregateContainerStateMap
}

func subtractCurrentContainerMemoryPeak(a *model.AggregateContainerState, container *model.ContainerState, now time.Time) {
	if now.Before(container.WindowEnd) {
		a.AggregateMemoryPeaks.SubtractSample(model.BytesFromMemoryAmount(container.GetMaxMemoryPeak()), 1.0, container.WindowEnd)
	}
}

func (writer *checkpointWriter) StoreCheckpoints(ctx context.Context, now time.Time, minCheckpoints int) error {
	recs := getRecommendationsToCheckpoint(writer.cluster.Recommendations)
	for _, rec := range recs {
		select {
		case <-ctx.Done():
		default:
		}

		if ctx.Err() != nil && minCheckpoints <= 0 {
			return ctx.Err()
		}

		aggregateContainerStateMap := buildAggregateContainerStateMap(rec, writer.cluster, now)
		for container, aggregateContainerState := range aggregateContainerStateMap {
			containerCheckpoint, err := aggregateContainerState.SaveToCheckpoint()
			if err != nil {
				klog.Errorf("Cannot serialize checkpoint for vpa %v container %v. Reason: %+v", rec.ID.RecommendationName, container, err)
				continue
			}
			checkpointName := fmt.Sprintf("%s-%s", rec.ID.RecommendationName, container)
			recommendationCheckpoint := recv1alpha1.RecommendationCheckpoint{
				ObjectMeta: metav1.ObjectMeta{Name: checkpointName},
				Spec: recv1alpha1.RecommendationCheckpointSpec{
					ContainerName:            container,
					RecommendationObjectName: rec.ID.RecommendationName,
				},
				Status: *containerCheckpoint,
			}
			err = util.CreateOrUpdateRecommendationCheckpoint(writer.recommendationCheckpointClient.RecommendationCheckpoints(rec.ID.Namespace), &recommendationCheckpoint)
			if err != nil {
				klog.Errorf("Cannot save Recommendation %s/%s checkpoint for %s. Reason: %+v",
					rec.ID.Namespace, recommendationCheckpoint.Spec.RecommendationObjectName, recommendationCheckpoint.Spec.ContainerName, err)
			} else {
				klog.V(3).Infof("Saved Recommendation %s/%s checkpoint for %s",
					rec.ID.Namespace, recommendationCheckpoint.Spec.RecommendationObjectName, recommendationCheckpoint.Spec.ContainerName)
				rec.CheckpointWritten = now
			}
			minCheckpoints-- // 每一次 RunOnce 只写入 10 个 checkpoint
		}
	}
	return nil
}
