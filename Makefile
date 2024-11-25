#vars
SMESHIMAGENAME=smesh-proxy
REPO=thebsdbox
SMESHIMAGEFULLNAME=${REPO}/${SMESHIMAGENAME}
DEMOIMAGEFULLNAME=${REPO}/demo
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

proxy:
	make build_proxy
	make push_proxy

build_proxy:
	    @go generate
		@docker build -t ${SMESHIMAGEFULLNAME} .

build_demo:
		@cd demo
		@docker build -t ${DEMOIMAGEFULLNAME} .

push_proxy:
	    @docker push ${SMESHIMAGEFULLNAME}

push_demo:
	    @docker push ${DEMOIMAGEFULLNAME}

all: build push