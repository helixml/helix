package api

import (
	"fmt"
	"io/ioutil"
	"sort"
	"strings"
)

// submit pool and pop a lease off it
func (apiHandler *HttpApiHandler) Get(request *PoolRequest) (*Lease, error) {

	socket, err := apiHandler.GetWebsocket()
	if err != nil {
		return nil, fmt.Errorf("Error initialising websocket: %s\n", err)
	}

	poolMeta := request.Meta
	if poolMeta == nil {
		poolMeta = make(map[string]string)
	}
	if poolSlot != "" {
		// Set meta for this lease, also drop any existing lease which
		// match this slot.
		poolMeta["slot"] = poolSlot
	}
	request.Meta = poolMeta

	fmt.Printf("Getting pool...\n")
	pool, err := apiHandler.CreatePool(request)
	if err != nil {
		return nil, fmt.Errorf("Error posting pools: %s\n", err)
	}
	fmt.Printf("Got pool (%+v)\n", pool.Meta)

	// pool may have already existed, in which case the pool slot might
	// not have been merged into it by the API. Check, and manually
	// update it if needed.
	_, ok := pool.Meta["slot"]
	if !ok {
		// will get added to below
		pool.Meta["slot"] = ""
	}
	if !containsSlot(pool.Meta["slot"], poolSlot) {
		newMeta := pool.Meta
		newMeta["slot"] = addSlotToList(pool.Meta["slot"], poolSlot)
		pool, err = apiHandler.UpdatePoolMeta(
			PoolState{
				PoolId: pool.Id,
				Meta:   newMeta,
			},
		)
		fmt.Printf("Updated pool meta --> %+v, got %+v\n", newMeta, pool)
		if err != nil {
			return nil, fmt.Errorf("error adding slot to pool: %s", err)
		}
	}

	if poolSlot != "" {
		// search all pools (skip the one we just created though!)
		pools, err := apiHandler.GetPools()
		if err != nil {
			return nil, fmt.Errorf("unable to get full list of pools: %s", err)
		}
		for _, p := range pools {
			if s, ok := p.Meta["slot"]; ok {
				// don't accidemetantally drop the pool we just created (id check)
				if containsSlot(s, poolSlot) && p.State != "deleting" && p.State != "deleted" && p.Id != pool.Id {
					// drop any pools that have slot collisions
					// (only supposed to be max 1, but race conditions
					// might mean >1 exist, so delete any that match)
					/*err = dropPool(out, errOut, p.Id, poolSlot)
					if err != nil {
						fmt.Printf(
							"Error dropping pool %s with matching (%s) slot %s: %s, continuing...\n",
							pool.Id[:7], s, poolSlot, err,
						)
					} else {
						fmt.Printf(
							"Dropped prior pool %s because it (%s) matched slot %s\n",
							pool.Id[:7], s, poolSlot,
						)
					}*/
				}
			}
		}
	}

	// we subscribe to pool events now we have the pool
	poolSubscription, err := socket.Subscribe(pool.GetSubscriptionChannel())
	if err != nil {
		return nil, fmt.Errorf("Error subscribing to pool events: %s\n", err)
	}

	fmt.Printf("Got pool (%s)\n", pool.Id)
	if pool.State == "deleting" || pool.State == "deleted" {
		poolId := pool.Id
		// resurrect pool if neccessary
		pool, err = apiHandler.UpdatePool(PoolState{
			PoolId: poolId,
			State:  "unknown", // will trigger backend to build it if necc.
		})
		if err != nil {
			return nil, fmt.Errorf("Error resurrecting pool %s: %s", poolId, err)
		}
	}

	if pool.State != "ready" {
		fmt.Printf("Waiting for pool to be ready...\n")
		stopLogsChan, err := streamPoolLogs(socket, pool.Id)
		if err != nil {
			return nil, fmt.Errorf("Error getting logs channel for pool: %s\n", err)
		}
		_, err = waitForPool(*apiHandler, poolSubscription, pool.Id)
		stopLogsChan <- true
		if err != nil {
			pool, _ = apiHandler.GetPool(pool.Id)
			return nil, fmt.Errorf("Error waiting for pool %s, check logs and retry at https://testfaster.ci/pools: %s\n", pool.Id, err)
		}
		fmt.Printf("Pool is now ready\n")
	}

	var lease *Lease = nil

	meta := map[string]string{}

	// Get pool so that we have leases filled in (response from
	// create pool may not have this)
	poolId := pool.Id
	pool, err = apiHandler.GetPool(poolId)
	if err != nil {
		return nil, fmt.Errorf("unable to get full filled in result for pool %s: %s", poolId, err)
	}

	if name != "" {
		meta["name"] = name
		// reuse existing lease with matching name
		for _, l := range pool.Leases {
			if n, ok := l.Meta["name"]; ok && n == name {
				lease = l
			}
		}
	}

	if slot != "" {
		// Set meta for this lease, also drop any existing lease which
		// match this slot.
		meta["slot"] = slot

		candidateDropLeases := []*Lease{}
		for _, l := range pool.Leases {
			if s, ok := l.Meta["slot"]; ok {
				if s == slot && l.State != "complete" {
					candidateDropLeases = append(candidateDropLeases, l)
				}
			}
		}
		// leave 2 most recent leases there.
		sort.SliceStable(candidateDropLeases, func(i, j int) bool {
			return candidateDropLeases[i].CreatedAt.Before(candidateDropLeases[j].CreatedAt)
		})
		// drop all but last two (leave two behind)
		for i := 0; i < len(candidateDropLeases)-retainSlots; i++ {
			//l := candidateDropLeases[i]
			// drop any leases that have slot collisions
			// (only supposed to be max n, but race conditions
			// might mean >n exist, so delete any that match)
			/*
				err = dropLease(out, errOut, pool.Id, l.Id)
				if err != nil {
					fmt.Printf(
						"Error dropping lease %s with matching slot %s: %s, continuing...\n",
						l.Id[:7], slot, err,
					)
				} else {
					fmt.Printf(
						"Dropped prior lease %s because it matched slot %s\n",
						l.Id[:7], slot,
					)
				}
			*/
		}
	}

	if lease == nil {
		fmt.Printf("Creating lease...\n")
		lease, err = apiHandler.CreateLease(&Lease{
			Pool:  pool.Id,
			State: "waiting",
			Meta:  meta,
		})
		if err != nil {
			return nil, fmt.Errorf("Error posting lease: %s\n", err)
		}
		fmt.Printf("Lease created (%s)\n", lease.Id)
	} else {
		fmt.Printf("Found existing lease (%s) with name %s\n", lease.Id, name)
	}

	fmt.Printf("\n=========================================\n")
	fmt.Printf("To connect to this VM, install and auth the cli from https://testfaster.ci/access_token then run:\n")
	fmt.Printf("    testctl ssh --pool %s --lease %s\n", pool.Id, lease.Id)
	fmt.Printf("=========================================\n\n")
	fmt.Printf("Waiting for lease to be assigned...\n")

	leaseSubscription, err := socket.Subscribe(lease.GetSubscriptionChannel())
	if err != nil {
		return nil, fmt.Errorf("Error subscribing for lease updates: %s\n", err)
	}

	leaseState, err := waitForLeaseAssigned(*apiHandler, leaseSubscription, pool.Id, lease.Id)
	if err != nil {
		return nil, fmt.Errorf("Error waiting for lease to be assigned: %s\n", err)
	}

	tempKubeconfig := `# Testfaster intermediate kubeconfig (recording lease info before VM is
# started, just enough for testctl get to drop the lease & pool if necc.)

##LEASE_ID=` + lease.Id + `
##POOL_ID=` + pool.Id + "\n"

	err = ioutil.WriteFile("kubeconfig", []byte(tempKubeconfig), 0644)
	if err != nil {
		return nil, fmt.Errorf("Error: could not write temp kubeconfig: %s\n", err)
	}

	fmt.Printf("\nNote: If you want to abort building the VM, you can now safely press ^C\n")
	fmt.Printf("and testctl get won't leak pools & VMs\n\n")

	// we are now waiting for the vm to start
	// let's hook into the vm logsss
	if leaseState == "assigned" {
		fmt.Printf("Waiting for lease to be ready...\n")
		lease, err = apiHandler.GetLease(pool.Id, lease.Id)
		if err != nil {
			return nil, fmt.Errorf("Error getting lease state: %s\n", err)
		}
		stopLogsChan, err := streamVmLogs(socket, pool.Id, lease.Vm)
		if err != nil {
			return nil, fmt.Errorf("Error getting logs channel for lease: %s\n", err)
		}
		_, err = waitForLeaseReady(*apiHandler, leaseSubscription, pool.Id, lease.Id)
		stopLogsChan <- true
		if err != nil {
			lease, _ = apiHandler.GetLease(pool.Id, lease.Id)
			return nil, fmt.Errorf("Error waiting for lease: %s\n%+v\n", err, lease)
		}
	}

	fmt.Printf("Lease is now ready\n")

	err = socket.Unsubscribe(pool.GetSubscriptionChannel())

	if err != nil {
		return nil, fmt.Errorf("Error unsubscribing to pool events: %s\n", err)
	}

	lease, err = apiHandler.GetLease(pool.Id, lease.Id)

	if err != nil {
		return nil, fmt.Errorf("Error getting lease state: %s\n", err)
	}

	err = ioutil.WriteFile("kubeconfig", []byte(lease.Kubeconfig), 0644)
	if err != nil {
		return nil, fmt.Errorf("Error: could not write to kubeconfig: %s\n", err)
	} else {
		fmt.Printf("Cluster acquired, now run:\n\n    export KUBECONFIG=$(pwd)/kubeconfig\n    kubectl get pods --all-namespaces\n\nConsider adding kubeconfig to .gitignore\n")
	}

	/*
		--------------------------------------------------
		$ testctl get -- with '.testfaster.yml' in place

		1. POST /api/v1/pools {config}
		resp --> {"id": "a1b2c3d"} [NB: may be reused if config matched existing pool]

		... as above

	*/
	return lease, nil
}

var keepExistingLease, keepExistingPool bool
var name string
var slot string
var poolSlot string
var retainSlots int

func containsSlot(metaSlots string, poolSlot string) bool {
	// The new convention is that the pool.meta["slots"] field contains a
	// semicolon-separated list of slots, rather than just a single slot. This
	// is a stringy list rather than an actual json list for backward
	// compatibility with pool slots that are strings. This function simply
	// tells you whether poolSlot is in metaSlots.
	if poolSlot == "" {
		return false
	}
	metaSlotsSlice := strings.Split(metaSlots, ";")
	for _, ms := range metaSlotsSlice {
		if ms == poolSlot {
			return true
		}
	}
	return false
}

func addSlotToList(slotList string, newSlot string) string {
	slotListSlice := strings.Split(slotList, ";")
	slotListSlice = append(slotListSlice, newSlot)
	return strings.Join(slotListSlice, ";")
}

func removeSlotFromList(slotList string, removeSlot string) string {
	slotListSlice := strings.Split(slotList, ";")
	newList := []string{}
	for _, s := range slotListSlice {
		if s != removeSlot {
			newList = append(newList, s)
		}
	}
	return strings.Join(newList, ";")
}
