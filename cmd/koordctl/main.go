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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"

	cachepodapis "gitlab.alibaba-inc.com/cache/api/pb/generated"
)

func main() {
	var requestFile string
	var certFile string
	timeout := 5 * time.Second
	pflag.StringVar(&requestFile, "f", "", "the request json file")
	pflag.StringVar(&certFile, "cert", "", "cert file")
	pflag.DurationVar(&timeout, "t", timeout, "the timeout of request")
	pflag.Parse()

	data, err := ioutil.ReadFile(requestFile)
	if err != nil {
		panic(err)
	}
	var request cachepodapis.AllocateCachedPodsRequest
	if err = json.Unmarshal(data, &request); err != nil {
		panic(err)
	}
	request.RequestId = string(uuid.NewUUID())
	request.Timeout = &metav1.Duration{
		Duration: timeout,
	}

	var creds credentials.TransportCredentials
	if certFile != "" {
		var err error
		creds, err = credentials.NewClientTLSFromFile(certFile, "scheduling.koordinator.sh")
		if err != nil {
			panic(err)
		}
	} else {
		creds = insecure.NewCredentials()
	}

	conn, err := grpc.Dial("127.0.0.1:10261", grpc.WithTransportCredentials(creds))
	if err != nil {
		panic(err)
	}
	client := cachepodapis.NewCacheSchedulerClient(conn)

	response, err := client.AllocateCachedPods(context.TODO(), &request)
	if err != nil {
		fmt.Printf("failed to request, err: %v\n", err)
	}
	if response != nil {
		data, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			panic(err)
		}
		fmt.Println("got response:")
		fmt.Println(string(data))
	}
}
