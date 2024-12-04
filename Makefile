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

proxy: build_proxy push_proxy

build_proxy:
	    @go generate
		@docker build -t ${SMESHIMAGEFULLNAME} .

push_proxy:
	    @docker push ${SMESHIMAGEFULLNAME}

demo: build_demo push_demo

build_demo:
		@docker build -t ${DEMOIMAGEFULLNAME} ./demo

push_demo:
	    @docker push ${DEMOIMAGEFULLNAME}

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

controller_manifest:
		@kubectl kustomize controller/deploy/ > ./manifests/controller.yaml

kind:
		@kubectl delete -f ./manifests/deployment_sidecar_secret.yaml; make build_proxy; kind load docker-image thebsdbox/smesh-proxy:v1; kubectl apply -f ./manifests/deployment_sidecar_secret.yaml

all: build push
