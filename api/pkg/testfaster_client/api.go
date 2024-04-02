package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-retryablehttp"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

type HttpApiHandler struct {
	endpoint    string
	accessToken string
	websocket   *WebSocket
}

type WebsocketSubscription struct {
	Channel         string
	IncomingChannel chan WebsocketEnvelope
}

type WebSocket struct {
	endpoint           string
	accessToken        string
	connection         *websocket.Conn
	subscriptions      map[string]*WebsocketSubscription
	dataMutex          *sync.Mutex
	sendMutex          *sync.Mutex
	subscriptionsMutex *sync.Mutex
}

type WebsocketEnvelope struct {
	Handler     string `json:"handler"`
	Channel     string `json:"channel"`
	MessageType string `json:"messageType"`
	Body        string `json:"body"`
}

func NewHttpApiHandler(endpoint, accessToken string) *HttpApiHandler {
	handler := &HttpApiHandler{
		endpoint:    endpoint,
		accessToken: accessToken,
	}
	return handler
}

type SocketImplementation interface {
	SendText(handler, channel, messageType, body string) error
	Subscribe(channel string) (*WebsocketSubscription, error)
	Unsubscribe(channel string) error
}

func (pool *Pool) GetSubscriptionChannel() string {
	return fmt.Sprintf("pool.%s", pool.Id)
}

func streamPoolLogs(socket SocketImplementation, id string) (chan bool, error) {
	return streamEntityLogs(socket, fmt.Sprintf("pool_logs.%s", id), "pool_logs")
}

func streamVmLogs(socket SocketImplementation, pool, id string) (chan bool, error) {
	return streamEntityLogs(socket, fmt.Sprintf("vm_logs.%s.%s", pool, id), "vm_logs")
}

func streamEntityLogs(socket SocketImplementation, channel, messageType string) (chan bool, error) {
	subscription, err := socket.Subscribe(channel)

	if err != nil {
		return nil, err
	}

	stopChan := make(chan bool)

	// wait for the pool to be ready via a websocket update
	go func() {
		for {
			select {
			case envelope := <-subscription.IncomingChannel:
				if envelope.Handler != "pubsubMessage" {
					continue
				}
				if envelope.MessageType == messageType {
					logMessage := &LogMessage{}
					err := json.Unmarshal([]byte(envelope.Body), &logMessage)
					if err != nil {
						fmt.Printf("error decoding log JSON: %s\n\n%s\n", err, envelope.Body)
					} else {
						if logMessage.Stream == "stdout" {
							fmt.Fprintf(os.Stdout, "%s\n", logMessage.Text)
						} else if logMessage.Stream == "stderr" {
							fmt.Fprintf(os.Stderr, "%s\n", logMessage.Text)
						}
					}
				}
			case <-stopChan:
				socket.Unsubscribe(channel)
				return
			}
		}
	}()

	return stopChan, nil
}

func (lease *Lease) GetSubscriptionChannel() string {
	return fmt.Sprintf("lease.%s.%s", lease.Pool, lease.Id)
}

func getLeaseLoader(apiHandler HttpApiHandler, poolId, leaseId string) func() (string, string, error) {
	return func() (string, string, error) {
		lease, err := apiHandler.GetLease(poolId, leaseId)
		if err != nil {
			return "", "", err
		}
		if lease.State == "error" {
			return "", lease.Status, fmt.Errorf("wait failed, state was error\n")
		} else {
			return lease.State, lease.Status, nil
		}
	}
}

func getLeaseProcessor() func(envelope WebsocketEnvelope) (string, error) {
	return func(envelope WebsocketEnvelope) (string, error) {
		lease := &Lease{}
		err := json.Unmarshal([]byte(envelope.Body), lease)
		if err != nil {
			return "", fmt.Errorf("Error decoding JSON body %s\n\n%s\n", err.Error(), envelope.Body)
		}
		if lease.State == "error" {
			return "", fmt.Errorf("wait failed, state was error\n")
		} else {
			return lease.State, nil
		}
	}
}

func waitForLease(apiHandler HttpApiHandler, poolSubscription *WebsocketSubscription, pool, id string, isReady func(string) bool) (string, error) {

	getError := func(state string) error {
		if state == "error" || state == "complete" || state == "timeout" {
			return fmt.Errorf("wait failed, state was %s", state)
		} else {
			return nil
		}
	}

	waiter := NewEntityWaiter(
		poolSubscription,
		getLeaseLoader(apiHandler, pool, id),
		getLeaseProcessor(),
		isReady,
		getError,
		fmt.Sprintf("pool.%s.%s", pool, id),
		"lease_update",
	)
	return waiter.wait()
}

func (waiter *EntityWaiterSocket) wait() error {
	// wait for the pool to be ready via a websocket update
	go func() {
		for {
			select {
			case envelope := <-waiter.poolSubscription.IncomingChannel:
				if envelope.Handler != "pubsubMessage" || envelope.MessageType != waiter.messageType {
					continue
				}
				state, err := waiter.processor(envelope)
				if err != nil {
					waiter.errorChan <- err
				}
				stateError := waiter.getError(state)
				if stateError != nil {
					waiter.errorChan <- stateError
				} else if waiter.isReady(state) {
					waiter.readyChan <- state
				}
			case <-waiter.stopChan:
				return
			}
		}
	}()

	return nil
}
func (waiter EntityWaiterApi) check() (bool, string, string, error) {
	state, status, err := waiter.loader()
	if err != nil {
		return false, state, status, err
	}
	stateError := waiter.getError(state)
	if stateError != nil {
		return false, state, status, stateError
	} else if waiter.isReady(state) {
		return true, state, status, nil
	}

	return false, state, status, nil
}

func (waiter *EntityWaiter) wait() (string, error) {

	// do an initial check on the state in case we miss the socket update
	isReady, state, _, err := waiter.apiWaiter.check()

	if err != nil {
		return "", err
	}

	if isReady {
		return state, nil
	}

	err = waiter.apiWaiter.wait()

	if err != nil {
		return "", err
	}

	err = waiter.socketWaiter.wait()

	if err != nil {
		return "", err
	}

	for {
		select {
		case err := <-waiter.apiWaiter.errorChan:
			go func() {
				waiter.apiWaiter.stopChan <- true
				waiter.socketWaiter.stopChan <- true
			}()
			return "", err
		case err := <-waiter.socketWaiter.errorChan:
			go func() {
				waiter.apiWaiter.stopChan <- true
				waiter.socketWaiter.stopChan <- true
			}()
			return "", err
		case state := <-waiter.apiWaiter.readyChan:
			go func() {
				waiter.apiWaiter.stopChan <- true
				waiter.socketWaiter.stopChan <- true
			}()
			return state, nil
		case state := <-waiter.socketWaiter.readyChan:
			go func() {
				waiter.apiWaiter.stopChan <- true
				waiter.socketWaiter.stopChan <- true
			}()
			return state, nil
		}
	}
}

func (waiter EntityWaiterApi) wait() error {
	go func() {
		ticker := time.Tick(1 * time.Second)
		for {
			select {
			case <-ticker:
				isReady, state, status, err := waiter.check()
				if err != nil {
					waiter.errorChan <- err
				} else if isReady {
					waiter.readyChan <- state
				} else {
					fmt.Printf("Not ready yet (%s, %s)...\n", state, status)
				}
			case <-waiter.stopChan:
				fmt.Printf("Read from stopChan, stopping polling.\n")
				return
			}
		}
	}()

	return nil
}

func waitForLeaseAssigned(apiHandler HttpApiHandler, poolSubscription *WebsocketSubscription, pool, id string) (string, error) {
	isReady := func(state string) bool {
		return state == "ready" || state == "assigned"
	}
	return waitForLease(apiHandler, poolSubscription, pool, id, isReady)
}

func waitForLeaseReady(apiHandler HttpApiHandler, poolSubscription *WebsocketSubscription, pool, id string) (string, error) {
	isReady := func(state string) bool {
		return state == "ready"
	}
	return waitForLease(apiHandler, poolSubscription, pool, id, isReady)
}

func getPoolLoader(apiHandler HttpApiHandler, id string) func() (string, string, error) {
	return func() (string, string, error) {
		pool, err := apiHandler.GetPool(id)
		if err != nil {
			return "", "", err
		}
		if pool.State == "error" {
			return "", pool.Status, fmt.Errorf("wait failed, state was error\n")
		} else {
			return pool.State, pool.Status, nil
		}
	}
}

func getPoolProcessor() func(envelope WebsocketEnvelope) (string, error) {
	return func(envelope WebsocketEnvelope) (string, error) {
		pool := &Pool{}
		err := json.Unmarshal([]byte(envelope.Body), pool)
		if err != nil {
			return "", fmt.Errorf("Error decoding JSON body %s\n\n%s\n", err.Error(), envelope.Body)
		}
		if pool.State == "error" {
			return "", fmt.Errorf("wait failed, state was error\n")
		} else {
			return pool.State, nil
		}
	}
}

func waitForPool(apiHandler HttpApiHandler, poolSubscription *WebsocketSubscription, id string) (string, error) {

	isReady := func(state string) bool {
		return state == "ready"
	}

	getError := func(state string) error {
		if state != "error" {
			return nil
		}
		return fmt.Errorf("wait failed, state was: %s\n", state)
	}

	waiter := NewEntityWaiter(
		poolSubscription,
		getPoolLoader(apiHandler, id),
		getPoolProcessor(),
		isReady,
		getError,
		fmt.Sprintf("pool.%s", id),
		"pool_update",
	)

	return waiter.wait()
}

type EntityWaiterApi struct {
	loader    func() (string, string, error)
	errorChan chan error
	readyChan chan string
	stopChan  chan bool
	url       string
	isReady   func(string) bool
	getError  func(string) error
}

type EntityWaiterSocket struct {
	poolSubscription *WebsocketSubscription
	processor        func(WebsocketEnvelope) (string, error)
	errorChan        chan error
	readyChan        chan string
	stopChan         chan bool
	channel          string
	messageType      string
	isReady          func(string) bool
	getError         func(string) error
}

type EntityWaiter struct {
	apiWaiter    *EntityWaiterApi
	socketWaiter *EntityWaiterSocket
}

func NewEntityApiWaiter(
	loader func() (string, string, error),
	isReady func(string) bool,
	getError func(string) error,
) *EntityWaiterApi {
	waiter := &EntityWaiterApi{
		loader:    loader,
		isReady:   isReady,
		getError:  getError,
		errorChan: make(chan error),
		readyChan: make(chan string),
		stopChan:  make(chan bool),
	}
	return waiter
}

func NewEntitySocketWaiter(
	poolSubscription *WebsocketSubscription,
	processor func(WebsocketEnvelope) (string, error),
	isReady func(string) bool,
	getError func(string) error,
	channel string,
	messageType string,
) *EntityWaiterSocket {
	waiter := &EntityWaiterSocket{
		poolSubscription: poolSubscription,
		processor:        processor,
		isReady:          isReady,
		getError:         getError,
		channel:          channel,
		messageType:      messageType,
		errorChan:        make(chan error),
		readyChan:        make(chan string),
		stopChan:         make(chan bool),
	}
	return waiter
}

func NewEntityWaiter(
	poolSubscription *WebsocketSubscription,
	loader func() (string, string, error),
	processor func(WebsocketEnvelope) (string, error),
	isReady func(string) bool,
	getError func(string) error,
	channel string,
	messageType string,
) *EntityWaiter {
	waiter := &EntityWaiter{
		apiWaiter:    NewEntityApiWaiter(loader, isReady, getError),
		socketWaiter: NewEntitySocketWaiter(poolSubscription, processor, isReady, getError, channel, messageType),
	}
	return waiter
}

func (handler *HttpApiHandler) GetWebsocket() (SocketImplementation, error) {
	if handler.websocket != nil {
		return handler.websocket, nil
	}
	socket, err := NewWebsocket(handler.endpoint, handler.accessToken)
	if err != nil {
		return nil, err
	}
	handler.websocket = socket
	return socket, nil
}

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("%s took %s", name, elapsed)
}

func (handler *HttpApiHandler) Request(method string, path string, body interface{}, result interface{}) error {

	if os.Getenv("DEBUG_HTTP") != "" {
		defer timeTrack(time.Now(), fmt.Sprintf("%s %s", method, path))
	}

	url := handler.endpoint + path

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 10
	client := retryClient.StandardClient()

	var err error
	var resp *http.Response

	hasBody := body != nil && (method == "POST" || method == "PUT")

	var reader io.Reader

	var b []byte
	if hasBody {
		b, err = json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewBuffer(b)
	}

	if os.Getenv("DEBUG_HTTP_ALL") != "" || (hasBody && os.Getenv("DEBUG_HTTP") != "") {
		fmt.Println("--> request Method:", method)
		fmt.Println("--> request Url:", url)
		fmt.Printf("--> request Body: %s\n", string(b))
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return err
	}

	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}

	if handler.accessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", handler.accessToken))
	}

	resp, err = client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		errorBody := &ErrorResponse{}
		err = json.Unmarshal(respBody, &errorBody)
		if err == nil {
			return fmt.Errorf("%d %s", resp.StatusCode, errorBody.Error)
		} else {
			return fmt.Errorf("bad status code %d", resp.StatusCode)
		}
	}

	if os.Getenv("DEBUG_HTTP_ALL") != "" || (hasBody && os.Getenv("DEBUG_HTTP") != "") {
		fmt.Println("--> response Status:", resp.Status)
		fmt.Println("--> response Headers:", resp.Header)
		fmt.Println("--> response Body:", string(respBody))
	}

	if result != nil {
		bodyString := string(respBody)
		err = json.Unmarshal(respBody, &result)
		if err != nil {
			log.Printf("Error decoding response '%s'", bodyString)
		}
	}
	return err
}

func (handler *HttpApiHandler) PingBackend() (*Backend, error) {
	backend := &Backend{}
	err := handler.Request("PUT", "/api/v1/backend/ping", nil, &backend)
	if err != nil {
		log.Printf("Error checking access token: %s\n", err)
		return nil, err
	}
	return backend, nil
}

func (handler *HttpApiHandler) GetPools() ([]*Pool, error) {
	poolArray := []*Pool{}
	err := handler.Request("GET", "/api/v1/pools", nil, &poolArray)
	if err != nil {
		return nil, err
	}
	return poolArray, nil
}

func (handler *HttpApiHandler) CreatePool(request *PoolRequest) (*Pool, error) {
	pool := &Pool{}
	err := handler.Request("POST", "/api/v1/pools", request, pool)
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func (handler *HttpApiHandler) GetPool(id string) (*Pool, error) {
	pool := Pool{}
	err := handler.Request("GET", fmt.Sprintf("/api/v1/pools/%s", id), nil, &pool)
	if err != nil {
		return nil, err
	}
	return &pool, nil
}

func (handler *HttpApiHandler) UpdatePool(update PoolState) (*Pool, error) {
	updatedPool := &Pool{}
	err := handler.Request("PUT", fmt.Sprintf("/api/v1/pools/%s/state", update.PoolId), update, &updatedPool)
	if err != nil {
		return nil, err
	}
	return updatedPool, nil
}

func (handler *HttpApiHandler) UpdatePoolMeta(update PoolState) (*Pool, error) {
	updatedPool := &Pool{}
	err := handler.Request("PUT", fmt.Sprintf("/api/v1/pools/%s/meta", update.PoolId), update, &updatedPool)
	if err != nil {
		return nil, err
	}
	return updatedPool, nil
}

func (handler *HttpApiHandler) CreateVm(vm *Vm) (*Vm, error) {
	createdVm := &Vm{}
	err := handler.Request("POST", fmt.Sprintf("/api/v1/pools/%s/vms", vm.Pool), vm, &createdVm)
	if err != nil {
		return nil, err
	}
	return createdVm, nil
}

func (handler *HttpApiHandler) UpdateVm(update VmState) (*Vm, error) {
	log.Printf("[UpdateVm] updating with %+v", update)
	updatedVm := &Vm{}
	err := handler.Request("PUT", fmt.Sprintf("/api/v1/pools/%s/vms/%s/state", update.PoolId, update.VmId), update, &updatedVm)
	if err != nil {
		return nil, err
	}
	return updatedVm, nil
}

func (handler *HttpApiHandler) DeleteVm(poolId, vmId string) error {
	return handler.Request("DELETE", fmt.Sprintf("/api/v1/pools/%s/vms/%s", poolId, vmId), nil, nil)
}

func (handler *HttpApiHandler) GetLease(poolId, leaseId string) (*Lease, error) {
	lease := Lease{}
	err := handler.Request("GET", fmt.Sprintf("/api/v1/pools/%s/leases/%s", poolId, leaseId), nil, &lease)
	if err != nil {
		return nil, err
	}
	return &lease, nil
}

func (handler *HttpApiHandler) CreateLease(lease *Lease) (*Lease, error) {
	createdLease := &Lease{}
	err := handler.Request("POST", fmt.Sprintf("/api/v1/pools/%s/leases", lease.Pool), lease, &createdLease)
	if err != nil {
		return nil, err
	}
	return createdLease, nil
}

func (handler *HttpApiHandler) UpdateLease(update LeaseState) (*Lease, error) {
	updatedLease := &Lease{}
	err := handler.Request("PUT", fmt.Sprintf("/api/v1/pools/%s/leases/%s/state", update.PoolId, update.LeaseId), update, &updatedLease)
	if err != nil {
		return nil, err
	}
	return updatedLease, nil
}

func (handler *HttpApiHandler) PostData(url string, data interface{}) error {
	return handler.Request(
		"POST",
		url,
		data,
		nil,
	)
}

type Backend struct {
	Id string `json:"id"`
}

type Connection struct {
	Endpoint    string
	AccessToken string
}

type PortMapping struct {
	From int `json:"from"`
	To   int `json:"to"`
}

type PortMappings struct {
	KubePort PortMapping   `json:"kube_port"`
	AppPorts []PortMapping `json:"app_ports"`
}

type PoolState struct {
	PoolId string             `json:"pool_id"`
	State  string             `json:"state"` // "building", "ready", "deleting", "deleted", "error"
	Status string             `json:"status"`
	Vms    map[string]VmState `json:"vms"`
	Meta   map[string]string  `json:"meta"`
}

type VmState struct {
	PoolId       string        `json:"pool_id"`
	VmId         string        `json:"vm_id"` // UUID not api id
	Ip           string        `json:"ip"`
	State        string        `json:"state"` // "dicovering", "off", "starting", "running", "deleting", "deleted", "error"
	Status       string        `json:"status"`
	PortMappings *PortMappings `json:"port_mappings"`
}

type LeaseState struct {
	PoolId      string    `json:"pool_id"`
	LeaseId     string    `json:"lease_id"`
	VmId        string    `json:"vm_id"` // id from ignite
	State       string    `json:"state"` // "waiting", "assigned", "ready", "complete", "timeout", "error"
	Status      string    `json:"status"`
	Kubeconfig  string    `json:"kubeconfig"`
	AssignedAt  time.Time `json:"assigned_at" yaml:"assigned_at"`
	ActivatedAt time.Time `json:"activated_at" yaml:"activated_at"`
}

type Secret struct {
	Id    string `json:"id,omitempty" yaml:"id"`
	Name  string `json:"name" yaml:"name"`
	Value string `json:"value" yaml:"value"`
}

type Pool struct {
	Id                 string            `json:"id,omitempty" yaml:"id"`
	CreatedAt          time.Time         `json:"created_at,omitempty" yaml:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at,omitempty" yaml:"updated_at"`
	Team               string            `json:"team,omitempty" yaml:"team,omitempty"`
	Name               string            `json:"name" yaml:"name"`
	Type               string            `json:"type" yaml:"type"`
	DesiredState       string            `json:"desired_state,omitempty" yaml:"desired_state,omitempty"`
	State              string            `json:"state" yaml:"state"`                                   // machine readable short string
	Status             string            `json:"status,omitempty" yaml:"status"`                       // human readable error string or longer description with handy debug info
	BackendType        string            `json:"backend_type,omitempty" yaml:"backend_type,omitempty"` // are we scheduled onto team or public backends?
	AccessType         string            `json:"access_type,omitempty" yaml:"access_type,omitempty"`   // is this pool shared between any other teams (in the case of try-faster where one pool config will be used by lots of different users)
	BaseConfigHash     string            `json:"base_config_hash" yaml:"base_config_hash"`
	RuntimeConfigHash  string            `json:"runtime_config_hash" yaml:"runtime_config_hash"`
	CombinedConfigHash string            `json:"combined_config_hash" yaml:"combined_config_hash"`
	KernelImageHash    string            `json:"kernel_image_hash" yaml:"kernel_image_hash"`
	OSImageHash        string            `json:"os_image_hash" yaml:"os_image_hash"`
	Config             PoolConfig        `json:"config" yaml:"config"`
	Meta               map[string]string `json:"meta" yaml:"meta"`
	Vms                []*Vm             `json:"vms,omitempty" yaml:"vms"`
	Leases             []*Lease          `json:"leases,omitempty" yaml:"leases"`
	Secrets            []Secret          `json:"secrets,omitempty" yaml:"secrets"`
}

type Vm struct {
	Id           string            `json:"id" yaml:"id"`
	Uuid         string            `json:"uuid" yaml:"uuid"` // backend-generated uuid
	CreatedAt    time.Time         `json:"created_at" yaml:"created_at"`
	Pool         string            `json:"pool" yaml:"pool"`
	Backend      string            `json:"backend" yaml:"backend"`
	State        string            `json:"state" yaml:"state"`
	Status       string            `json:"status,omitempty" yaml:"status"` // human readable error string or longer description with handy debug info
	Ip           string            `json:"ip" yaml:"ip"`
	Meta         map[string]string `json:"meta" yaml:"meta"`
	PortMappings *PortMappings     `json:"port_mappings" yaml:"port_mappings"`
}

type Lease struct {
	Id                string            `json:"id" yaml:"id"`
	CreatedAt         time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at" yaml:"updated_at"`
	AssignedAt        time.Time         `json:"assigned_at" yaml:"assigned_at"`
	ActivatedAt       time.Time         `json:"activated_at" yaml:"activated_at"`
	Pool              string            `json:"pool" yaml:"pool"`
	Backend           string            `json:"backend" yaml:"backend"`
	Vm                string            `json:"vm" yaml:"vm"`
	State             string            `json:"state" yaml:"state"`
	Status            string            `json:"status,omitempty" yaml:"status"` // human readable error string or longer description with handy debug info
	Kubeconfig        string            `json:"kubeconfig" yaml:"kubeconfig"`
	Timeout           string            `json:"timeout" yaml:"timeout"`
	AllocationTimeout string            `json:"allocation_timeout" yaml:"allocation_timeout"`
	Meta              map[string]string `json:"meta" yaml:"meta"`
}

type BaseConfig struct {
	OsDockerfile        string   `json:"os_dockerfile,omitempty" yaml:"os_dockerfile"`
	OsImage             string   `json:"os_image,omitempty" yaml:"os_image"`
	KernelDockerfile    string   `json:"kernel_dockerfile,omitempty" yaml:"kernel_dockerfile"`
	KernelImage         string   `json:"kernel_image,omitempty" yaml:"kernel_image"`
	DockerBakeScript    string   `json:"docker_bake_script" yaml:"docker_bake_script"`
	PreloadDockerImages []string `json:"preload_docker_images" yaml:"preload_docker_images" jsonschema:"oneof_type=null;array"`
	PrewarmScript       string   `json:"prewarm_script" yaml:"prewarm_script"`
	// e.g. as passed to minikube start --kubernetes-version
	// https://minikube.sigs.k8s.io/docs/commands/start/#minikube-start
	// XXX This should arguably be in RuntimeConfig
	KubernetesVersion string `json:"kubernetes_version" yaml:"kubernetes_version"`
}

type RuntimeConfig struct {
	Cpus   int    `json:"cpus" yaml:"cpus"`     // e.g. 2
	Memory string `json:"memory" yaml:"memory"` // e.g. "2GB"
	Disk   string `json:"disk" yaml:"disk"`     // e.g. "30GB"
}

type LaunchButton struct {
	Title string `json:"title" yaml:"title"`
	// specify either port and path, or url
	Port int    `json:"port,omitempty" yaml:"port,omitempty"`
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
	Url  string `json:"url,omitempty" yaml:"url,omitempty"`
}

type LaunchConfig struct {
	Title    string         `json:"title" yaml:"title"`
	Buttons  []LaunchButton `json:"buttons,omitempty" yaml:"buttons,omitempty"`
	Homepage string         `json:"homepage" yaml:"homepage"`
}

// also wot user sends when they type 'testctl get'
type PoolConfig struct {
	Base                          BaseConfig    `json:"base" yaml:"base"`
	Runtime                       RuntimeConfig `json:"runtime" yaml:"runtime"`
	Launch                        LaunchConfig  `json:"launch,omitempty" yaml:"launch,omitempty"`
	Name                          string        `json:"name" yaml:"name"`
	AutoDetectPreloadDockerImages bool          `json:"autodetect_preload_docker_images" yaml:"autodetect_preload_docker_images"`
	PrewarmPoolSize               int           `json:"prewarm_pool_size" yaml:"prewarm_pool_size"`
	MaxPoolSize                   int           `json:"max_pool_size" yaml:"max_pool_size"`
	DefaultLeaseTimeout           string        `json:"default_lease_timeout" yaml:"default_lease_timeout"`
	DefaultLeaseAllocationTimeout string        `json:"default_lease_allocation_timeout" yaml:"default_lease_allocation_timeout"`
	PoolSleepTimeout              string        `json:"pool_sleep_timeout,omitempty" yaml:"pool_sleep_timeout"`
	KubernetesWaitForPods         bool          `json:"kubernetes_wait_for_pods" yaml:"kubernetes_wait_for_pods"`
	Shared                        bool          `json:"shared,omitempty" yaml:"shared,omitempty"`
}

// When we make pools, they don't have vms or leases or state or status, when we read them they do.
type PoolRequest struct {
	Id                 string            `json:"id" yaml:"id"`
	CreatedAt          time.Time         `json:"created_at" yaml:"created_at"`
	Name               string            `json:"name" yaml:"name"`
	Type               string            `json:"type" yaml:"type"`
	BaseConfigHash     string            `json:"base_config_hash" yaml:"base_config_hash"`
	RuntimeConfigHash  string            `json:"runtime_config_hash" yaml:"runtime_config_hash"`
	CombinedConfigHash string            `json:"combined_config_hash" yaml:"combined_config_hash"`
	Config             PoolConfig        `json:"config" yaml:"config"`
	Meta               map[string]string `json:"meta" yaml:"meta"`
	Default            bool              `json:"default,omitempty" yaml:"default"` // If user gives no file on the cli, we create a pool config with Default: true
}

type LogMessage struct {
	Stream string `json:"stream"` // "stdout", "stderr"
	Text   string `json:"text"`
	Order  int    `json:"order"`
}

// implement interface so we can check the state
// of pools, vms and leases in a common way
type EntityStateReader interface {
	GetState() string
}

func (pool Pool) GetState() string {
	return pool.State
}

func (vm Vm) GetState() string {
	return vm.State
}

func (lease Lease) GetState() string {
	return lease.State
}
