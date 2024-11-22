# Quick demo of unencrypted TCP

## Quick deployment
kubectl delete -f ./deploy.yaml; docker build -t demo/demo:v1 .; kind load docker-image demo/demo:v1;

kubectl apply -f ./deploy.yaml
