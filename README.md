# lilysaas

 * what are the core database schema "things"
 * what is the plan for taking payments?
 * what are the key actions?

## entities

 * auth
   * use keycloak
 * model
   * json file to start with
   * a list of things that lilypad can run
 * job
   * inference
   * training (fine tunings)
     * starts with an existing model
     * produces a new model

## plan

This is to have a hello world that you can login to.

 * docker compose stack
   * keycloak
   * frontend (react & vite)
   * api (go)
   * postgres


## dev

You need the following installed:

 * docker
 * docker-compose
 * [abigen](https://geth.ethereum.org/docs/getting-started/installing-geth)

You also need the lilypad repo cloned alongside this one.

### smart contract interface

When the smart contract in lilypad changes - checkout latest main of lilypad and then:

```bash
./stack generate-golang-bindings
```

This will re-create the `api/pkg/contract/Modicum.go`
