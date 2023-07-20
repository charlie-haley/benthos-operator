# benthos-operator

Kubernetes operator that acts as a control plane for managing the lifecycle of Benthos pipelines.

> **⚠️ this project is currently in a pre-alpha state.**

## ✨Features

- ♻️ Complete lifecycle management of Benthos pipelines
- ⚙️ Automated rollout on config changes
- ⚖️ Configurable scaling

## Getting Started

The operator is still in very early stages, there's currently no Helm Chart or released Docker image. However, it does work (I promise), just don't use it in production....

If you'd like to test it out, you can follow the [Developer Guide](./docs/developer-guide.md) to get the operator deployed.

Once it's running, you can deploy a pipeline with the following Custom Resource:

```
---
apiVersion: streaming.benthos.dev/v1alpha1
kind: BenthosPipeline
metadata:
  name: benthospipeline-sample
spec:
  replicas: 2
  config: |
    input:
      generate:
        mapping: root = "woof"
        interval: 5s
        count: 0

    pipeline:
      processors:
        - mapping: root = content().uppercase()

    output:
      stdout: {}
```

Once you've deployed your pipeline resource, you can check the current state of the pipeline.

```bash
kubectl get benthospipelines

NAME                     READY   PHASE     REPLICAS   AVAILABLE   AGE
benthospipeline-sample   true    Running   2          2           62s
```

The operator will also perform a rollout of the pods when the config is changed.

```
NAME                     READY   PHASE      REPLICAS   AVAILABLE   AGE
benthospipeline-sample   true    Updating   15         12          61m
```

## Contributing

If you'd like to contribute, [please take a look at our outstanding issues](https://github.com/charlie-haley/benthos-operator/issues)
