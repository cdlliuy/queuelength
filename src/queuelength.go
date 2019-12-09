package main

import (
	"bytes"
	"encoding/json"
	"errors"
	_ "expvar"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"strings"
	"time"
)

type job struct {
	duration time.Duration
}

type Credentials struct {
	Appid    string `json:"app_id"`
	Username string `json:"username"`
	Password string `json:"password"`
	URL      string `json:"url"`
}

type Userprovided struct {
	Name        string      `json:"name"`
	Credentials Credentials `json:"credentials"`
}

type VCAPservices struct {
	Userprovideds []Userprovided `json:"user-provided"`
}

type CustomMetrics struct {
	InstanceIndex int       `json:"instance_index"`
	Metrics       []Metrics `json:"metrics"`
}

type Metrics struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
	Unit  string `json:"unit"`
}

func fetchAutoScalerSetting() (Credentials, error) {

	var vcapservice VCAPservices
	var credentials Credentials

	jsonBlob := []byte(os.Getenv("VCAP_SERVICES"))

	err := json.Unmarshal(jsonBlob, &vcapservice)
	if err != nil {
		return credentials, err
	}
	for _, entry := range vcapservice.Userprovideds {
		if strings.HasPrefix(entry.Name, "autoscaler") {
			return entry.Credentials, nil
		}
	}
	return credentials, errors.New("Missing Auto-Scaling credentials")
}

func reportToAutoScaler(url string, credentials Credentials, instanceIndex, metricValue int) error {
	metric := CustomMetrics{
		InstanceIndex: instanceIndex,
		Metrics: []Metrics{
			{
				Name:  "queuelength",
				Value: metricValue,
				Unit:  "",
			},
		},
	}

	var body io.Reader
	jsonByte, err := json.Marshal(metric)
	if err != nil {
		return err
	}
	body = bytes.NewBuffer(jsonByte)

	req, err := http.NewRequest("POST", url, body)
	req.SetBasicAuth(credentials.Username, credentials.Password)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("Fail to emit metrics with status %s, %s", resp.Status, string(raw))
	}

	return nil

}

func emitMetrics(jobs chan job, done chan bool) error {
	instanceIndex, err := strconv.Atoi(os.Getenv("CF_INSTANCE_INDEX"))
	if err != nil {
		return err
	}
	credentials, err := fetchAutoScalerSetting()
	if err != nil {
		return err
	}
	emitURL := fmt.Sprintf("%s/v1/apps/%s/metrics", credentials.URL, credentials.Appid)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			fmt.Println("Emit Done!")
			return nil
		case <-ticker.C:
			err := reportToAutoScaler(emitURL, credentials, instanceIndex, len(jobs))
			if err != nil {
				fmt.Printf("%v\n", err)
			}
			fmt.Println("Current queue length is ", len(jobs))
		}
	}
}

func doWork(id int, j job) {
	fmt.Printf("worker%d: will work for %v seconds\n", id, j.duration.Seconds())
	time.Sleep(j.duration)
}

func requestHandler(jobs chan job, w http.ResponseWriter, r *http.Request) {
	delay := r.URL.Query().Get("delay")

	duration, err := time.ParseDuration(delay)
	if err != nil {
		http.Error(w, "Bad delay value: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Create Job and push the work onto the jobCh.
	job := job{duration}
	go func() {
		fmt.Printf("add a new job: duration %s \n", job.duration)
		jobs <- job
	}()

	// Render success.
	w.WriteHeader(http.StatusCreated)
	return
}

func main() {
	var (
		maxQueueSize = flag.Int("max_queue_size", 100000, "The size of job queue")
		maxWorkers   = flag.Int("max_workers", 5, "The number of workers to start")
		port         = flag.String("port", "8080", "The server port")
		emitDone     = make(chan bool)
	)
	flag.Parse()

	// create job channel
	jobs := make(chan job, *maxQueueSize)

	// create workers
	for i := 1; i <= *maxWorkers; i++ {
		go func(i int) {
			for j := range jobs {
				doWork(i, j)
			}
		}(i)
	}

	go emitMetrics(jobs, emitDone)

	// handler for adding jobs
	http.HandleFunc("/work", func(w http.ResponseWriter, r *http.Request) {
		requestHandler(jobs, w, r)
	})
	http.HandleFunc("/emitStop", func(w http.ResponseWriter, r *http.Request) {
		close(emitDone)
	})
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}
