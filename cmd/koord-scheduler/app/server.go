/*
Copyright 2022 The Koordinator Authors.

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

// Package app implements a Server object for running the scheduler.
package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	goruntime "runtime"

	"github.com/spf13/cobra"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	genericapifilters "k8s.io/apiserver/pkg/endpoints/filters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/server"
	genericfilters "k8s.io/apiserver/pkg/server/filters"
	"k8s.io/apiserver/pkg/server/healthz"
	"k8s.io/apiserver/pkg/server/mux"
	"k8s.io/apiserver/pkg/server/routes"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/leaderelection"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/component-base/configz"
	"k8s.io/component-base/logs"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/component-base/term"
	"k8s.io/component-base/version"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
	scheduleroptions "k8s.io/kubernetes/cmd/kube-scheduler/app/options"
	"k8s.io/kubernetes/pkg/scheduler"
	kubeschedulerconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/latest"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	"k8s.io/kubernetes/pkg/scheduler/metrics/resources"
	"k8s.io/kubernetes/pkg/scheduler/profile"

	schedulerserverconfig "github.com/koordinator-sh/koordinator/cmd/koord-scheduler/app/config"
	"github.com/koordinator-sh/koordinator/cmd/koord-scheduler/app/options"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/eventhandlers"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/services"
	utilroutes "github.com/koordinator-sh/koordinator/pkg/util/routes"

	frameworkextunified "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/unified"
)

// Option configures a framework.Registry.
type Option func(frameworkext.ExtendedHandle, runtime.Registry) error

// NewSchedulerCommand creates a *cobra.Command object with default parameters and registryOptions
func NewSchedulerCommand(schedulingHooks []frameworkext.SchedulingPhaseHook, registryOptions ...Option) *cobra.Command {
	opts := options.NewOptions()

	cmd := &cobra.Command{
		Use: "koord-scheduler",
		Long: `The Koordinator scheduler is a control plane process which assigns
Pods to Nodes. The scheduler implements based on kubernetes scheduling framework.
On the basis of compatibility with community scheduling capabilities, it provides 
richer advanced scheduling capabilities to address scheduling needs in co-located 
scenarios,ensuring the runtime quality of different workloads and users' demands 
for cost reduction and efficiency enhancement.
`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := runCommand(cmd, opts, schedulingHooks, registryOptions...); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
		},
		Args: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if len(arg) > 0 {
					return fmt.Errorf("%q does not take any arguments, got %q", cmd.CommandPath(), args)
				}
			}
			return nil
		},
	}

	nfs := opts.Flags
	verflag.AddFlags(nfs.FlagSet("global"))
	globalflag.AddGlobalFlags(nfs.FlagSet("global"), cmd.Name())
	frameworkext.AddFlags(nfs.FlagSet("extend"))
	fs := cmd.Flags()
	for _, f := range nfs.FlagSets {
		fs.AddFlagSet(f)
	}

	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cliflag.SetUsageAndHelpFunc(cmd, *nfs, cols)

	cmd.MarkFlagFilename("config", "yaml", "yml", "json")

	return cmd
}

// runCommand runs the scheduler.
func runCommand(cmd *cobra.Command, opts *options.Options, schedulingHooks []frameworkext.SchedulingPhaseHook, registryOptions ...Option) error {
	verflag.PrintAndExitIfRequested()
	cliflag.PrintFlags(cmd.Flags())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		stopCh := server.SetupSignalHandler()
		<-stopCh
		cancel()
	}()

	cc, sched, extendedHandle, err := Setup(ctx, opts, schedulingHooks, registryOptions...)
	if err != nil {
		return err
	}

	return Run(ctx, cc, sched, extendedHandle)
}

// Run executes the scheduler based on the given configuration. It only returns on error or when context is done.
func Run(ctx context.Context, cc *schedulerserverconfig.CompletedConfig, sched *scheduler.Scheduler, extendedHandle frameworkext.ExtendedHandle) error {
	// To help debugging, immediately log version
	klog.V(1).InfoS("Starting Koordinator Scheduler version", "version", version.Get())

	// Configz registration.
	if cz, err := configz.New("componentconfig"); err == nil {
		cz.Set(cc.ComponentConfig)
	} else {
		return fmt.Errorf("unable to register configz: %s", err)
	}

	// Prepare the event broadcaster.
	cc.EventBroadcaster.StartRecordingToSink(ctx.Done())

	// Setup healthz checks.
	var checks []healthz.HealthChecker
	if cc.ComponentConfig.LeaderElection.LeaderElect {
		checks = append(checks, cc.LeaderElection.WatchDog)
	}

	waitingForLeader := make(chan struct{})
	isLeader := func() bool {
		select {
		case _, ok := <-waitingForLeader:
			// if channel is closed, we are leading
			return !ok
		default:
			// channel is open, we are waiting for a leader
			return false
		}
	}

	// Start up the healthz server.
	if cc.InsecureServing != nil {
		separateMetrics := cc.InsecureMetricsServing != nil
		handler := buildHandlerChain(newAPIHandler(&cc.ComponentConfig, cc.InformerFactory, cc.ServicesEngine, sched, isLeader, separateMetrics, checks...), nil, nil)
		if err := cc.InsecureServing.Serve(handler, 0, ctx.Done()); err != nil {
			return fmt.Errorf("failed to start insecure server: %v", err)
		}
	}
	if cc.InsecureMetricsServing != nil {
		handler := buildHandlerChain(newMetricsHandler(&cc.ComponentConfig, cc.InformerFactory, isLeader), nil, nil)
		if err := cc.InsecureMetricsServing.Serve(handler, 0, ctx.Done()); err != nil {
			return fmt.Errorf("failed to start metrics server: %v", err)
		}
	}
	if cc.SecureServing != nil {
		handler := buildHandlerChain(newAPIHandler(&cc.ComponentConfig, cc.InformerFactory, cc.ServicesEngine, sched, isLeader, false, checks...), cc.Authentication.Authenticator, cc.Authorization.Authorizer)
		// TODO: handle stoppedCh returned by c.SecureServing.Serve
		if _, err := cc.SecureServing.Serve(handler, 0, ctx.Done()); err != nil {
			// fail early for secure handlers, removing the old error loop from above
			return fmt.Errorf("failed to start secure server: %v", err)
		}
	}

	// Start all informers.
	cc.InformerFactory.Start(ctx.Done())
	cc.KoordinatorSharedInformerFactory.Start(ctx.Done())

	// Wait for all caches to sync before scheduling.
	cc.InformerFactory.WaitForCacheSync(ctx.Done())
	cc.KoordinatorSharedInformerFactory.WaitForCacheSync(ctx.Done())

	// If leader election is enabled, runCommand via LeaderElector until done and exit.
	if cc.LeaderElection != nil {
		cc.LeaderElection.Callbacks = leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				close(waitingForLeader)
				go extendedHandle.Run()
				sched.Run(ctx)
			},
			OnStoppedLeading: func() {
				select {
				case <-ctx.Done():
					// We were asked to terminate. Exit 0.
					klog.Info("Requested to terminate. Exiting.")
					os.Exit(0)
				default:
					// We lost the lock.
					klog.Exitf("leaderelection lost")
				}
			},
		}
		leaderElector, err := leaderelection.NewLeaderElector(*cc.LeaderElection)
		if err != nil {
			return fmt.Errorf("couldn't create leader elector: %v", err)
		}

		leaderElector.Run(ctx)

		return fmt.Errorf("lost lease")
	}

	// Leader election is disabled, so runCommand inline until done.
	close(waitingForLeader)
	go extendedHandle.Run()
	sched.Run(ctx)
	return fmt.Errorf("finished without leader elect")
}

// buildHandlerChain wraps the given handler with the standard filters.
func buildHandlerChain(handler http.Handler, authn authenticator.Request, authz authorizer.Authorizer) http.Handler {
	requestInfoResolver := &apirequest.RequestInfoFactory{}
	failedHandler := genericapifilters.Unauthorized(scheme.Codecs)

	handler = genericapifilters.WithAuthorization(handler, authz, scheme.Codecs)
	handler = genericapifilters.WithAuthentication(handler, authn, failedHandler, nil)
	handler = genericapifilters.WithRequestInfo(handler, requestInfoResolver)
	handler = genericapifilters.WithCacheControl(handler)
	handler = genericfilters.WithHTTPLogging(handler)
	handler = genericfilters.WithPanicRecovery(handler, requestInfoResolver)

	return handler
}

func installMetricHandler(pathRecorderMux *mux.PathRecorderMux, informers informers.SharedInformerFactory, isLeader func() bool) {
	configz.InstallHandler(pathRecorderMux)
	pathRecorderMux.Handle("/metrics", legacyregistry.HandlerWithReset())

	resourceMetricsHandler := resources.Handler(informers.Core().V1().Pods().Lister())
	pathRecorderMux.HandleFunc("/metrics/resources", func(w http.ResponseWriter, req *http.Request) {
		if !isLeader() {
			return
		}
		resourceMetricsHandler.ServeHTTP(w, req)
	})
}

func installProfilingHandler(pathRecorderMux *mux.PathRecorderMux, enableContentionProfiling bool) {
	routes.Profiling{}.Install(pathRecorderMux)
	if enableContentionProfiling {
		goruntime.SetBlockProfileRate(1)
	}
	// NOTE: Use utilroutes.DebugFlags instead of k8s.io/apiserver/pkg/server/routes.DebugFlags
	//  as using the latter will print a useless stack when installing multiple flags
	debugFlags := utilroutes.NewDebugFlags(pathRecorderMux)
	debugFlags.Install("v", utilroutes.StringFlagPutHandler(logs.GlogSetter))
	debugFlags.Install("s", utilroutes.StringFlagPutHandler(frameworkext.DebugScoresSetter))
	debugFlags.Install("f", utilroutes.StringFlagPutHandler(frameworkext.DebugFiltersSetter))
}

// newMetricsHandler builds a metrics server from the config.
func newMetricsHandler(config *kubeschedulerconfig.KubeSchedulerConfiguration, informers informers.SharedInformerFactory, isLeader func() bool) http.Handler {
	pathRecorderMux := mux.NewPathRecorderMux("koord-scheduler")
	installMetricHandler(pathRecorderMux, informers, isLeader)
	if config.EnableProfiling {
		installProfilingHandler(pathRecorderMux, config.EnableContentionProfiling)
	}
	return pathRecorderMux
}

// newAPIHandler creates a healthz server from the config, and will also
// embed the metrics handler if the healthz and metrics address configurations
// are the same.
func newAPIHandler(config *kubeschedulerconfig.KubeSchedulerConfiguration, informers informers.SharedInformerFactory, engine *services.Engine, sched *scheduler.Scheduler, isLeader func() bool, separateMetrics bool, checks ...healthz.HealthChecker) http.Handler {
	pathRecorderMux := mux.NewPathRecorderMux("koord-scheduler")
	healthz.InstallHandler(pathRecorderMux, checks...)
	if !separateMetrics {
		installMetricHandler(pathRecorderMux, informers, isLeader)
	}
	if config.EnableProfiling {
		installProfilingHandler(pathRecorderMux, config.EnableContentionProfiling)
	}
	services.InstallAPIHandler(pathRecorderMux, engine, sched, isLeader)
	return pathRecorderMux
}

func getRecorderFactory(cc *schedulerserverconfig.CompletedConfig) profile.RecorderFactory {
	return func(name string) events.EventRecorder {
		return cc.EventBroadcaster.NewRecorder(name)
	}
}

// WithPlugin creates an Option based on plugin name and factory. Please don't remove this function: it is used to register out-of-tree plugins,
// hence there are no references to it from the kubernetes scheduler code base.
func WithPlugin(name string, factory runtime.PluginFactory) Option {
	return func(handle frameworkext.ExtendedHandle, registry runtime.Registry) error {
		return registry.Register(name, frameworkext.PluginFactoryProxy(handle, factory))
	}
}

// Setup creates a completed config and a scheduler based on the command args and options
func Setup(ctx context.Context, opts *options.Options, schedulingHooks []frameworkext.SchedulingPhaseHook, outOfTreeRegistryOptions ...Option) (*schedulerserverconfig.CompletedConfig, *scheduler.Scheduler, frameworkext.ExtendedHandle, error) {
	if cfg, err := latest.Default(); err != nil {
		return nil, nil, nil, err
	} else {
		opts.ComponentConfig = cfg
	}

	if errs := opts.Validate(); len(errs) > 0 {
		return nil, nil, nil, utilerrors.NewAggregate(errs)
	}

	c, err := opts.Config()
	if err != nil {
		return nil, nil, nil, err
	}

	// Get the completed config
	cc := c.Complete()

	// NOTE(joseph): K8s scheduling framework does not provide extension point for initialization.
	// Currently, only by copying the initialization code and implementing custom initialization.
	extendedHandle, err := frameworkext.NewExtendedHandle(
		frameworkext.WithServicesEngine(cc.ServicesEngine),
		frameworkext.WithKoordinatorClientSet(cc.KoordinatorClient),
		frameworkext.WithKoordinatorSharedInformerFactory(cc.KoordinatorSharedInformerFactory),
		frameworkext.WithSharedListerFactory(frameworkextunified.NewOverQuotaSharedLister),
	)
	if err != nil {
		return nil, nil, nil, err
	}

	outOfTreeRegistry := make(runtime.Registry)
	for _, option := range outOfTreeRegistryOptions {
		if err := option(extendedHandle, outOfTreeRegistry); err != nil {
			return nil, nil, nil, err
		}
	}

	recorderFactory := getRecorderFactory(&cc)
	completedProfiles := make([]kubeschedulerconfig.KubeSchedulerProfile, 0)
	// Create the scheduler.
	sched, err := scheduler.New(cc.Client,
		cc.InformerFactory,
		recorderFactory,
		ctx.Done(),
		scheduler.WithComponentConfigVersion(cc.ComponentConfig.TypeMeta.APIVersion),
		scheduler.WithKubeConfig(cc.KubeConfig),
		scheduler.WithProfiles(cc.ComponentConfig.Profiles...),
		scheduler.WithLegacyPolicySource(cc.LegacyPolicySource),
		scheduler.WithPercentageOfNodesToScore(cc.ComponentConfig.PercentageOfNodesToScore),
		scheduler.WithFrameworkOutOfTreeRegistry(outOfTreeRegistry),
		scheduler.WithPodMaxBackoffSeconds(cc.ComponentConfig.PodMaxBackoffSeconds),
		scheduler.WithPodInitialBackoffSeconds(cc.ComponentConfig.PodInitialBackoffSeconds),
		scheduler.WithExtenders(cc.ComponentConfig.Extenders...),
		scheduler.WithParallelism(cc.ComponentConfig.Parallelism),
		scheduler.WithBuildFrameworkCapturer(func(profile kubeschedulerconfig.KubeSchedulerProfile) {
			// Profiles are processed during Framework instantiation to set default plugins and configurations. Capturing them for logging
			completedProfiles = append(completedProfiles, profile)
		}),
	)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := scheduleroptions.LogOrWriteConfig(opts.WriteConfigTo, &cc.ComponentConfig, completedProfiles); err != nil {
		return nil, nil, nil, err
	}

	// TODO(joseph): Some extensions can also be made in the future,
	//  such as replacing some interfaces in Scheduler to implement custom logic

	// extend framework to hook run plugin functions
	extendedFrameworkFactory := frameworkext.NewFrameworkExtenderFactory(extendedHandle, schedulingHooks...)
	for k, v := range sched.Profiles {
		sched.Profiles[k] = extendedFrameworkFactory.New(v)
	}

	schedulerInternalHandler := &eventhandlers.SchedulerInternalHandlerImpl{
		Scheduler: sched,
	}
	eventhandlers.AddScheduleEventHandler(sched, schedulerInternalHandler, extendedHandle)
	eventhandlers.AddReservationErrorHandler(sched, schedulerInternalHandler, extendedHandle)

	return &cc, sched, extendedHandle, nil
}
