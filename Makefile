#vars
VERSION=v1
SMESHIMAGENAME=smesh-proxy
REPO=thebsdbox
SMESHIMAGEFULLNAME=${REPO}/${SMESHIMAGENAME}:${VERSION}
DEMOIMAGEFULLNAME=${REPO}/demo:${VERSION}
WATCHERIMAGEFULLNAME=${REPO}/watcher:${VERSION}
CONTROLLERIMAGEFULLNAME=${REPO}/smesh-controller:${VERSION}

.PHONY: help build push all

help:
	    @echo "Makefile arguments:"
	    @echo ""
	    @echo "alpver - Alpine Version"
	    @echo "kctlver - kubectl version"
	    @echo ""
	    @echo "Makefile commands:"
	    @echo "build"
	    @echo "push"
	    @echo "all"

.DEFAULT_GOAL := all

sidecar: build_proxy push_proxy

build_sidecar:
	    @go generate ./pkg/manager
		@docker build -t ${SMESHIMAGEFULLNAME} -f ./Dockerfile-sidecar . 

push_sidecar:
	    @docker push ${SMESHIMAGEFULLNAME}

kind_sidecar:
		@kind load docker-image thebsdbox/smesh-proxy:v1

demo: build_demo push_demo

build_demo:
		@docker build -t ${DEMOIMAGEFULLNAME} ./demo

push_demo:
	    @docker push ${DEMOIMAGEFULLNAME}

kind_demo:
		@kind load docker-image thebsdbox/demo:v1

watcher: build_watcher push_watcher

build_watcher: 
		@docker build -t ${WATCHERIMAGEFULLNAME} ./watcher

push_watcher:
	    @docker push ${WATCHERIMAGEFULLNAME}

controller: build_controller push_controller

build_controller: 
		@docker build -t ${CONTROLLERIMAGEFULLNAME} ./controller

push_controller:
	    @docker push ${CONTROLLERIMAGEFULLNAME}

kind_controller: 
		@kind load docker-image thebsdbox/smesh-controller:v1

controller_manifest:
		@kubectl kustomize controller/deploy/ > ./manifests/controller.yaml

kind:
		@kubectl delete -f ./manifests/deployment_sidecar_secret.yaml; make build_proxy; kind load docker-image thebsdbox/smesh-proxy:v1; kubectl apply -f ./manifests/deployment_sidecar_secret.yaml

all: build push
