.PHONY: image
image:
	kubectl dev build -t docker.io/warmmetal/csi-configmap:v0.2.1
	kubectl dev build -t docker.io/warmmetal/csi-configmap:latest

.PHONY: test
test:
	kubectl dev build -t docker.io/warmmetal/csi-configmap-test:v0.1.0 -f test.dockerfile test/integration

.PHONY: unit
unit:
	kubectl dev build -t csi-configmap-test:unit -f test.dockerfile

.PHONY: sanity
sanity:
	kubectl dev build -t local.test/csi-driver-cm-test:sanity test/sanity
	kubectl delete --ignore-not-found -f test/sanity/manifest.yaml
	kubectl apply --wait -f test/sanity/manifest.yaml
	kubectl -n cliapp-system wait --for=condition=complete job/csi-driver-cm-sanity-test

.PHONY: e2e
e2e:
	cp $(shell minikube ssh-key)* test/e2e/
	kubectl dev build -t local.test/csi-driver-cm-test:e2e test/e2e
	kubectl delete --ignore-not-found -f test/e2e/manifest.yaml
	kubectl apply --wait -f test/e2e/manifest.yaml
	kubectl -n cliapp-system wait --timeout=30m --for=condition=complete job/csi-driver-cm-e2e-test