# mTLS using cert-manager

When you enable mTLS in the operator using the following configuration, the operator asks cert-manager to generate some certificates for you. Cert-manager will then take care to renew them.

```yaml
  mTLS:
    provider: cert-manager
    internode:
      enabled: true
    frontend:
      enabled: true
    certificatesDuration:
      rootCACertificate: 2h
      intermediateCAsCertificates: 1h30m
      clientCertificates: 1h
      frontendCertificate: 1h
      internodeCertificate: 1h
    refreshInterval: 5m
```

## Overview

Here is a diagram of cert-manager's resources created by the operator and their hierarchy:

![diagram](/assets/mtls-certmanager.png)

## Operator connections

The operator creates its own client connections to the cluster, for instance to reconcile `TemporalNamespace` or `TemporalSchedule` resources. The address and certificate it uses depend on the cluster configuration:

- When the internal frontend is enabled (`spec.services.internalFrontend.enabled: true`), the operator connects to the internal frontend. The internal frontend is served using the internode mTLS settings, so the operator authenticates using the internode certificate when internode mTLS is enabled, and connects without TLS otherwise — even if frontend mTLS is enabled.
- Otherwise, the operator connects to the public frontend, using the frontend certificate when frontend mTLS is enabled.

When [authorization](https://docs.temporal.io/self-hosted-guide/security#authorization) is configured on the cluster, prefer enabling the internal frontend: temporal applies the noop claim mapper and authorizer to connections going through the internal frontend, so the operator doesn't need any grant in your authorizer. Enabling internode mTLS alongside the internal frontend is recommended, otherwise the operator's traffic to the internal frontend is unencrypted.

