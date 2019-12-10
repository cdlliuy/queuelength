# Sample Application for CF Autoscaling with Custom Metrics

This repo is used to provide a sample application to demostrate the CF Autoscaling capability with Custom metrics.  

## Prerequisite
You need to read through the blog post [Bring your own metrics to scale Cloud Foundry application](https://www.ibm.com/cloud/blog/bring-your-own-metrics-to-autoscale-your-ibm-cloud-foundry-applications) first to understand the scenario addressed by this repo. 

However,  the article introduces how to use UI dashboard to play with custom metric, but I will use CLI to achieve the same goal.  To learn more about App-autoscaler concept and its CLI usage, please read:

* [Cloud Foundry App-Autoscaler](https://github.com/cloudfoundry/app-autoscaler/blob/develop/docs/Readme.md)
* [Cloud Foundry Autoscaling on IBM Cloud](https://cloud.ibm.com/docs/cloud-foundry-public?topic=cloud-foundry-public-autoscale_cloud_foundry_apps)


## Getting start

*  Clone this repo to your local machine
```
git clone git@github.com:cdlliuy/queuelength.git
```

*  Push the source code to Cloud Foundry as an application.  Assuming the name is "queuelength"
```
cd queuelength
cf push queuelength -p ./src
```

* Attach autoscaler policy to the application
```
cf aasp queuelength policy.json
```

* Create autoscaler crendential for the application
```
cf create-autoscaling-credential queuelength --output queuelength.json
```

* Create an user-provided-service for this credential and bind the service to the application.
```
cf create-user-provided-service autoscaler-metric-service -p queuelength.json
cf bind-service queuelength autoscaler-metric-service
```
Note: the service name must have a prefix `autoscaler` since we will search for service crendential name with theis designed prefix

* Tailing logs to check the current queue depth
```
cf logs queuelength | grep "Current queue length"
```

* In another teminal,  add workload to the target application
```
for i in {1..1000}; do curl "http://queuelength.<domain>/work?delay=5s"; done
```
OR, if you have `vegeta` installed, you can kick off the workload with vegeta with a fine-gained control:
```
echo "GET http://queuelength.<domain>/work?delay=2s" | vegeta attack -duration=1200s -rate=3 | tee result.bin | vegeta report
```


* Retrieve autoscaler metrics to verify the custom metric reporting
```
>>> cf asm queuelength queuelength

Retrieving aggregated queuelength metrics for app queuelength...
Metrics Name     	Value     	Timestamp
queuelength      	173       	2019-12-09T13:56:41+08:00
queuelength      	156       	2019-12-09T13:56:00+08:00
queuelength      	139       	2019-12-09T13:55:20+08:00
```

* Retrieve autoscaler history to verify the scaling actions
```
>>> cf ash queuelength
Retrieving scaling event history for app queuelength...
Scaling Type     	Status        	Instance Changes     	Time                          	Action                                                      	Error
dynamic          	succeeded     	1->2                 	2019-12-09T13:56:59+08:00     	+1 instance(s) because queuelength >= 20 for 60 seconds
```

