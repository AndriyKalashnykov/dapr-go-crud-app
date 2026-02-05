# Example Dapr applications

This repo contains a set of simple golang applications that use Dapr and show some of it's various features.

> Pre-requisites: 

* Kubernetes cluster
* Install [ko.build](https://ko.build/) with <br> 
  `go install github.com/google/ko@latest`

## Setup
```
dapr init -k
make deploy
```

### References

* [Original crude app](https://github.com/famarting/crud-app)