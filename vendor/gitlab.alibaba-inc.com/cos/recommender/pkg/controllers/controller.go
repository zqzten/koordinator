package controllers

import (
	"flag"
	"net/http"
	"os"
	"time"

	kruiseclientset "github.com/openkruise/kruise-api/client/clientset/versioned"
	kruiseinformers "github.com/openkruise/kruise-api/client/informers/externalversions"
	"golang.org/x/time/rate"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"gitlab.alibaba-inc.com/cos/recommender/pkg/common"
	conf "gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/config"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/metrics"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommendationprofile"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/healthcheck"
	"gitlab.alibaba-inc.com/cos/recommender/pkg/controllers/recommender/history"
	unifiedclientset "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned"
	unifiedinformers "gitlab.alibaba-inc.com/cos/unified-resource-api/client/informers/externalversions"
)

var (
	config = common.NewConfiguration()
)

func init() {
	config.InitFlags(flag.CommandLine)
}

type historyProviderOption struct {
	// todo change to enum
	UseCheckpoint  bool
	PrometheusConf history.PrometheusHistoryProviderConfig
}

type Option struct {
	Scheme                             *runtime.Scheme
	ConfigMapNamespace                 string
	ConfigMapName                      string
	RecommendationProfileQPS           int
	RecommendationProfileBurst         int
	RecommendationProfileMaxConcurrent int
	Manager                            manager.Manager
	FailureRateLimiter                 workqueue.RateLimiter
	InformerFactory                    informers.SharedInformerFactory
}

type ControllerManager interface {
	Start(stopCh <-chan struct{})
	HealthCheck() http.Handler
}

type controllerManager struct {
	restConfig             *rest.Config
	controllerOpt          Option
	historyOpt             historyProviderOption
	healthCheck            *healthcheck.HealthCheck
	unifiedInformerFactory unifiedinformers.SharedInformerFactory
	kruiseInformerFactory  kruiseinformers.SharedInformerFactory
}

func NewControllerManager(restConfig *rest.Config, opt Option) ControllerManager {

	unifiedClientset := unifiedclientset.NewForConfigOrDie(restConfig)
	unifiedInformerFactory := unifiedinformers.NewSharedInformerFactory(unifiedClientset, 0)

	metrics.RegisterRecommendation()
	metrics.RegisterRecDetail()
	metrics.Register()

	historyOpt := historyProviderOption{
		UseCheckpoint: config.Storage != "prometheus",
	}
	if !historyOpt.UseCheckpoint {
		promQueryTimeout, _ := time.ParseDuration(config.QueryTimeout)
		historyOpt.PrometheusConf = history.PrometheusHistoryProviderConfig{
			Address:                config.PrometheusAddress,
			QueryTimeout:           promQueryTimeout,
			HistoryLength:          config.HistoryLength,
			HistoryResolution:      config.HistoryResolution,
			PodLabelPrefix:         config.PodLabelPrefix,
			PodLabelsMetricName:    config.PodLabelsMetricName,
			PodNamespaceLabel:      config.PodNamespaceLable,
			PodNameLabel:           config.PodNameLabel,
			CtrNamespaceLabel:      config.CtrNamespaceLabel,
			CtrPodNameLabel:        config.CtrPodNameLabel,
			CtrNameLabel:           config.CtrNameLabel,
			CadvisorMetricsJobName: config.PrometheusJobName,
			Namespace:              apiv1.NamespaceAll,
		}
	}

	metricsFetcherInterval, _ := time.ParseDuration(config.MetricsFetcherInterval)
	healthCheck := healthcheck.NewHealthCheck(metricsFetcherInterval*5, true)

	cm := &controllerManager{
		restConfig:             restConfig,
		healthCheck:            healthCheck,
		historyOpt:             historyOpt,
		controllerOpt:          opt,
		unifiedInformerFactory: unifiedInformerFactory,
	}

	if config.EnableSupportKruiseWorkload {
		kruiseClientset := kruiseclientset.NewForConfigOrDie(restConfig)
		kruiseInformerFactory := kruiseinformers.NewSharedInformerFactory(kruiseClientset, 0)
		cm.kruiseInformerFactory = kruiseInformerFactory
	}

	return cm
}

func (c *controllerManager) Start(stopCh <-chan struct{}) {
	rateLimiter := rate.NewLimiter(rate.Limit(c.controllerOpt.RecommendationProfileQPS), c.controllerOpt.RecommendationProfileBurst)
	reconcilerOptions := controller.Options{
		MaxConcurrentReconciles: c.controllerOpt.RecommendationProfileMaxConcurrent,
		RateLimiter:             workqueue.NewMaxOfRateLimiter(c.controllerOpt.FailureRateLimiter, &workqueue.BucketRateLimiter{Limiter: rateLimiter}),
	}
	waitInformerSynceds := []cache.InformerSynced{c.controllerOpt.InformerFactory.Core().V1().ConfigMaps().Informer().HasSynced,
		c.controllerOpt.InformerFactory.Core().V1().ConfigMaps().Informer().HasSynced,
		c.controllerOpt.InformerFactory.Core().V1().Pods().Informer().HasSynced,
		c.controllerOpt.InformerFactory.Core().V1().Namespaces().Informer().HasSynced,
		c.controllerOpt.InformerFactory.Core().V1().ReplicationControllers().Informer().HasSynced,
		c.controllerOpt.InformerFactory.Apps().V1().DaemonSets().Informer().HasSynced,
		c.controllerOpt.InformerFactory.Apps().V1().StatefulSets().Informer().HasSynced,
		c.controllerOpt.InformerFactory.Apps().V1().Deployments().Informer().HasSynced,
		c.controllerOpt.InformerFactory.Apps().V1().ReplicaSets().Informer().HasSynced,
		c.controllerOpt.InformerFactory.Batch().V1().Jobs().Informer().HasSynced,
		c.controllerOpt.InformerFactory.Batch().V1beta1().CronJobs().Informer().HasSynced,
		c.unifiedInformerFactory.Autoscaling().V1alpha1().RecommendationProfiles().Informer().HasSynced}
	if config.EnableSupportKruiseWorkload {
		waitInformerSynceds = append(waitInformerSynceds, c.kruiseInformerFactory.Apps().V1alpha1().CloneSets().Informer().HasSynced,
			c.kruiseInformerFactory.Apps().V1alpha1().DaemonSets().Informer().HasSynced,
			c.kruiseInformerFactory.Apps().V1beta1().StatefulSets().Informer().HasSynced,
			c.kruiseInformerFactory.Apps().V1alpha1().AdvancedCronJobs().Informer().HasSynced)

		go c.kruiseInformerFactory.Start(wait.NeverStop)
	}
	go c.unifiedInformerFactory.Start(wait.NeverStop)
	go c.controllerOpt.InformerFactory.Start(wait.NeverStop)
	klog.Infof("done informer factory")
	// wait for resource cache
	if !cache.WaitForCacheSync(stopCh, waitInformerSynceds...) {
		klog.Error("timed out waiting for resource cache to sync")
		os.Exit(1)
	}
	klog.Infof("done wait informer")

	klog.Infof("start RecommendationProfile reconcile ")
	redProfileReconciler := &recommendationprofile.RedProfileReconciler{
		Client:            c.controllerOpt.Manager.GetClient(),
		PodLister:         c.controllerOpt.InformerFactory.Core().V1().Pods().Lister(),
		NamespaceLister:   c.controllerOpt.InformerFactory.Core().V1().Namespaces().Lister(),
		RCLister:          c.controllerOpt.InformerFactory.Core().V1().ReplicationControllers().Lister(),
		RSLister:          c.controllerOpt.InformerFactory.Apps().V1().ReplicaSets().Lister(),
		DeploymentLister:  c.controllerOpt.InformerFactory.Apps().V1().Deployments().Lister(),
		StatefulSetLister: c.controllerOpt.InformerFactory.Apps().V1().StatefulSets().Lister(),
		DaemonSetLister:   c.controllerOpt.InformerFactory.Apps().V1().DaemonSets().Lister(),
		JobLister:         c.controllerOpt.InformerFactory.Batch().V1().Jobs().Lister(),
		CronJobLister:     c.controllerOpt.InformerFactory.Batch().V1beta1().CronJobs().Lister(),
		TimerEvents:       make(chan event.GenericEvent, 10),
	}
	if config.EnableSupportKruiseWorkload {
		redProfileReconciler.AdvancedStatefulSetLister = c.kruiseInformerFactory.Apps().V1beta1().StatefulSets().Lister()
		redProfileReconciler.AdvancedCronJobLister = c.kruiseInformerFactory.Apps().V1alpha1().AdvancedCronJobs().Lister()
		redProfileReconciler.AdvancedDaemonSetLister = c.kruiseInformerFactory.Apps().V1alpha1().DaemonSets().Lister()
		redProfileReconciler.CloneSetList = c.kruiseInformerFactory.Apps().V1alpha1().CloneSets().Lister()
	}
	if err := c.controllerOpt.Manager.Add(redProfileReconciler); err != nil {
		klog.Fatalf("unable to add runner for RecommendationProfile, error %v", err)
	}
	if err := redProfileReconciler.SetupWithManager(c.controllerOpt.Manager, reconcilerOptions); err != nil {
		klog.Fatalf("unable to create controller RecommendationProfile, error :%v", err)
	}

	klog.Infof("start PodRecommendation reconcile ")
	podRedProfileReconciler := &recommendationprofile.PodRedProfileReconciler{
		Scheme:           c.controllerOpt.Manager.GetScheme(),
		Client:           c.controllerOpt.Manager.GetClient(),
		RedProfileLister: c.unifiedInformerFactory.Autoscaling().V1alpha1().RecommendationProfiles().Lister(),
	}
	if err := podRedProfileReconciler.SetupWithManager(c.controllerOpt.Manager, reconcilerOptions); err != nil {
		klog.Errorf("unable to create controller PodRecommendation, err : %v", err)
		os.Exit(1)
	}

	klog.Infof("start Recommender controller")
	conf.InitializeConfig(c.controllerOpt.InformerFactory.Core().V1().ConfigMaps().Lister().ConfigMaps(c.controllerOpt.ConfigMapNamespace),
		c.controllerOpt.ConfigMapName)
	checkpointsGCInterval, _ := time.ParseDuration(config.CheckpointsGCInterval)
	recommender := recommender.NewRecommender(c.restConfig, checkpointsGCInterval, c.historyOpt.UseCheckpoint,
		apiv1.NamespaceAll, config.EnableSupportKruiseWorkload, c.controllerOpt.Scheme)

	if c.historyOpt.UseCheckpoint {
		recommender.GetClusterStateFeeder().InitFromCheckpoints()
	} else {
		provider, err := history.NewPrometheusHistoryProvider(c.historyOpt.PrometheusConf)
		if err != nil {
			klog.Fatalf("Could not initialize history provider: %v", err)
		}
		recommender.GetClusterStateFeeder().InitFromHistoryProvider(provider)
	}

	cycleRun := func() {
		recommender.RunOnce()
		c.healthCheck.UpdateLastActivity()
	}
	metricsFetcherInterval, _ := time.ParseDuration(config.MetricsFetcherInterval)
	go wait.Until(cycleRun, metricsFetcherInterval, stopCh)
}

func (c *controllerManager) HealthCheck() http.Handler {
	return c.healthCheck
}
