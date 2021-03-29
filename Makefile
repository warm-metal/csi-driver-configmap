.PHONY: image
image:
	kubectl dev build -t docker.io/warmmetal/csi-configmap:v0.2.0

.PHONY: test
test:
	kubectl dev build -t docker.io/warmmetal/csi-configmap-test:v0.1.0 -f test.dockerfile test

.PHONY: unit
unit:
	kubectl dev build -t csi-configmap-test:unit -f test.dockerfile