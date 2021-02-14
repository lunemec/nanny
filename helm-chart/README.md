# Nanny Helm Chart

Use this Helm chart to deploy Nanny on a Kubernetes cluster.

Minimal Helm deployment command:
```bash
helm upgrade nanny . \
  --install \
  --atomic \
  --version 0.4.2 \
  --namespace nanny \
  -f values.yaml \
  --set ingress.hosts[0].host=nanny.example.com \
  --set ingress.hosts[0].paths[0].path=/
```

All possible configuration values can be found inside the [values.yaml](values.yaml) file.