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

## running alongside lilypad in local dev

#### lilypad

First start lilypad and then bacalhau:

```bash
cd lilypad
./stack boot
```

Then we start bacalhau:

**NOTE** you will require the correct version of bacalhau - run through [this guide](https://github.com/bacalhau-project/lilypad/blob/fe9999b96d0920083ab3b1c4dbe4c647c5db36d3/CONTRIBUTING.md#bacalhau): 

```bash
./stack bacalhau-serve
```

Now we need 3 other terminals each with `LOG_LEVEL=debug`

```bash
./stack solver --server-url http://172.17.0.1:8080
```

so it reports an address accessible from inside docker (the default docker bridge ip - this will probably only work on linux? on mac maybe you can use `host.docker.internal`)


```bash
./stack mediator
```

```bash
./stack resource-provider
```

Now we switch to lilysaas and get it booted alongside lilypad.

#### lilysaas

We need to create an top level `.env` file like so

```
export WEB3_PRIVATE_KEY=XXX
export SERVICE_SOLVER=0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC
export SERVICE_MEDIATORS=0x90F79bf6EB2c4f870365E785982E1f101E93b906
```

You can get the WEB3_PRIVATE_KEY value with this command:

```bash
cat ../lilypad/.env | grep JOB_CREATOR_PRIVATE_KEY
```

Copy the value of this to be the `WEB3_PRIVATE_KEY` value in the `.env` file - the other values should be as shown.

```bash
docker-compose up -d
```

Then we exec into the api container and run the api server:

```bash
docker-compose exec api bash
go run . serve
```