package common

import "flag"

const (
	RecommenderConfigKey = "recommender-config"
)

type Configuration struct {
	// recommender 配置
	MetricsFetcherInterval      string
	CheckpointsGCInterval       string
	EnableSupportKruiseWorkload bool

	// 从prometheus中恢复数据的配置
	Storage           string
	PrometheusAddress string // 从prometheus查询指标的地址
	HistoryLength     string
	HistoryResolution string
	QueryTimeout      string
	// 与PodLabelsMetricName配合
	// 筛选出metrics中具有该前缀的label
	// 这些label可以用于recommendation进行匹配
	PodLabelPrefix string
	// 从该指标获取pod label信息，用于和recommendation进行匹配
	// 返回的指标必须包含pod name， ns， pod label 信息
	PodLabelsMetricName string
	PodNamespaceLable   string
	PodNameLabel        string
	CtrNamespaceLabel   string
	CtrPodNameLabel     string
	CtrNameLabel        string
	PrometheusJobName   string
}

func NewConfiguration() *Configuration {
	return &Configuration{
		MetricsFetcherInterval:      "1m",
		CheckpointsGCInterval:       "10m",
		EnableSupportKruiseWorkload: false,
		Storage:                     "checkpoint",
		HistoryLength:               "8d",
		HistoryResolution:           "1h",
		QueryTimeout:                "5m",

		PodLabelPrefix:      "label_",
		PodLabelsMetricName: "kube_pod_labels{}",
		PodNamespaceLable:   "namespace",
		PodNameLabel:        "pod_name",

		CtrNamespaceLabel: "namespace",
		CtrPodNameLabel:   "pod_name",
		CtrNameLabel:      "container",
		PrometheusJobName: "_arms/kubelet/cadvisor",
	}
}

func (c *Configuration) InitFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.MetricsFetcherInterval, "recommender-interval", c.MetricsFetcherInterval, `How often metrics should be fetched`)
	fs.StringVar(&c.CheckpointsGCInterval, "checkpointsGCInterval", c.CheckpointsGCInterval, `How often orphaned checkpoints should be garbage collected`)
	fs.BoolVar(&c.EnableSupportKruiseWorkload, "enabel-support-kruise-workload", c.EnableSupportKruiseWorkload, "recommend resources for the kruise workload , not support in default")
	fs.StringVar(&c.Storage, "storage", "checkpoint", `Specifies storage mode. Supported values: prometheus, checkpoint (default)`)
	fs.StringVar(&c.PrometheusAddress, "prometheus-address", "", `Where to reach for Prometheus metrics`)
	fs.StringVar(&c.HistoryLength, "history-length", c.HistoryLength, `How much time back prometheus have to be queried to get historical metrics`)
	fs.StringVar(&c.HistoryResolution, "history-resolution", c.HistoryResolution, `Resolution at which Prometheus is queried for historical metrics`)
	fs.StringVar(&c.QueryTimeout, "prometheus-query-timeout", c.QueryTimeout, `How long to wait before killing long queries`)

	fs.StringVar(&c.PodLabelPrefix, "pod-label-prefix", c.PodLabelPrefix, `Which prefix to look for pod labels in metrics`)
	fs.StringVar(&c.PodLabelsMetricName, "metric-for-pod-labels", c.PodLabelsMetricName, `Which metric to look for pod labels in metrics`)
	fs.StringVar(&c.PodNamespaceLable, "pod-namespace-label", c.PodNamespaceLable, `Label name to look for container names`)
	fs.StringVar(&c.PodNameLabel, "pod-name-label", c.PodNameLabel, `Label name to look for container names`)
	fs.StringVar(&c.CtrNamespaceLabel, "container-namespace-label", c.CtrNamespaceLabel, `Label name to look for container names`)
	fs.StringVar(&c.CtrPodNameLabel, "container-pod-name-label", c.CtrPodNameLabel, `Label name to look for container names`)
	fs.StringVar(&c.CtrNameLabel, "container-name-label", c.CtrNameLabel, `Label name to look for container names`)
	fs.StringVar(&c.PrometheusJobName, "prometheus-cadvisor-job-name", c.PrometheusJobName, `Name of the prometheus job name which scrapes the cAdvisor metrics`)
}
