#vars
VERSION=v1
SMESHIMAGENAME=smesh-proxy
REPO=thebsdbox
SMESHIMAGEFULLNAME=${REPO}/${SMESHIMAGENAME}:${VERSION}
DEMOIMAGEFULLNAME=${REPO}/demo:${VERSION}
WATCHERIMAGEFULLNAME=${REPO}/watcher:${VERSION}

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

demo: build_demo push_demo

watcher: build_watcher push_watcher

build_proxy:
	    @go generate
		@docker build -t ${SMESHIMAGEFULLNAME} .

build_demo:
		@docker build -t ${DEMOIMAGEFULLNAME} ./demo

push_proxy:
	    @docker push ${SMESHIMAGEFULLNAME}

push_demo:
	    @docker push ${DEMOIMAGEFULLNAME}

build_watcher: 
		@docker build -t ${WATCHERIMAGEFULLNAME} ./watcher

push_watcher:
	    @docker push ${WATCHERIMAGEFULLNAME}

all: build push
