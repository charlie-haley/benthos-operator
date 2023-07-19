# benthos-operator

Kubernetes operator that acts as a control plane for managing the lifecycle of Benthos pipelines.

> **⚠️ this project is currently in a pre-alpha state.**

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

## Contributing

If you'd like to contribute, [please take a look at our outstanding issues](https://github.com/charlie-haley/flowcontrol/issues)
