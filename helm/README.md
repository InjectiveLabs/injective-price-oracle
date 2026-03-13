# WS Price Oracle Helm Chart

This chart deploys websocket oracle instances grouped by provider type.

It uses `ExternalSecret` resources and reads from the `ClusterSecretStore` named `gcp-secret-store`.

## Values Structure

Define instances under `ws_oracle.<providerType>.<instanceName>`.

- Provider type is inferred from parent key (`chainlink`, `stork`, `web2Provider`).
- Do not set `type` inside each instance.
- Instance `name` is optional; if omitted it defaults to the map key (for example `chainlink_instance_1`).
- `cluster_name` is a top-level value shared by all instances.
- `env` is a top-level value (`testnet`/`mainnet`) used as a prefix for ExternalSecret target names.

## Common Required Fields

Set once globally:

- `env`
- `cluster_name`

Each instance should set:

- `enabled`
- `version`
- `env.chain.grpcEndpoint`
- `env.chain.tmEndpoint`
- `secret` (at least `refreshInterval`)

## Provider Setup Examples

### Chainlink

```yaml
env: testnet
cluster_name: dev
ws_oracle:
  chainlink:
    chainlink_instance_1:
      enabled: true
      version: latest
      env:
        chain:
          grpcEndpoint: grpc-endpoint:9090
          tmEndpoint: https://tm-endpoint:443
        chainlink:
          websocketUrl: wss://chainlink.example/ws
      secret:
        refreshInterval: 1h
        # Optional. Default key: "<env>-chainlink-chainlink_instance_1-streams"
        streams_override: some-other-secret-name
        # Optional. Default key: "<env>-chainlink-chainlink_instance_1-oracle-secrets"
        oracle_secrets_override: some-other-secret-name
```

### Stork

```yaml
env: testnet
cluster_name: dev
ws_oracle:
  stork:
    stork_instance_1:
      enabled: true
      version: latest
      env:
        chain:
          grpcEndpoint: grpc-endpoint:9090
          tmEndpoint: https://tm-endpoint:443
        stork:
          websocketUrl: wss://stork.example/ws
          websocketSubscribeMessage: '{"type":"subscribe"}'
      secret:
        refreshInterval: 1h
        # Optional. Default key: "<env>-stork-stork_instance_1-oracle-secrets"
        oracle_secrets_override: some-other-secret-name
```

### Web2 Provider

```yaml
env: testnet
cluster_name: dev
ws_oracle:
  web2Provider:
    web2_instance_1:
      enabled: true
      version: latest
      env:
        chain:
          grpcEndpoint: grpc-endpoint:9090
          tmEndpoint: https://tm-endpoint:443
      secret:
        refreshInterval: 1h
        # Optional. Default key: "<env>-web2Provider-web2_instance_1-oracle-api-keys"
        api_keys_override: some-other-secret-name
        # Optional. Default key: "<env>-web2Provider-web2_instance_1-oracle-secrets"
        oracle_secrets_override: psome-other-secret-name
```

## Render

```bash
helm template ws-price-oracle ./helm -f ./helm/values.yaml
```

ExternalSecret-backed secret names are prefixed with `env`, for example:

- `testnet-chainlink-main-oracle-secrets`
- `mainnet-stork-main-oracle-secrets`

Default ExternalSecret `dataFrom.key` values are:

- Oracle secrets: `<env>-<type>-<name>-oracle-secrets`
- Web2 api keys: `<env>-<type>-<name>-oracle-api-keys`

## Google Secret Manager Examples

`ExternalSecret` uses `dataFrom.extract`, so each GSM secret value should be a JSON object.

### 1) Chainlink oracle secret

Default name:

- `<env>-chainlink-<name>-oracle-secrets`

Example (`env=testnet`, `name=main`):

```bash
gcloud secrets create testnet-chainlink-main-oracle-secrets --replication-policy="automatic"
printf '%s' '{
  "ORACLE_COSMOS_PK": "your-private-key",
  "CHAINLINK_API_KEY": "your-chainlink-api-key",
  "CHAINLINK_API_SECRET": "your-chainlink-api-secret"
}' | gcloud secrets versions add testnet-chainlink-main-oracle-secrets --data-file=-
```

If keyring is enabled, also include:

- `ORACLE_COSMOS_FROM_PASSPHRASE`

### 2) Stork oracle secret

Default name:

- `<env>-stork-<name>-oracle-secrets`

Example (`env=testnet`, `name=main`):

```bash
gcloud secrets create testnet-stork-main-oracle-secrets --replication-policy="automatic"
printf '%s' '{
  "ORACLE_COSMOS_PK": "your-private-key",
  "STORK_WEBSOCKET_HEADER": "secret-injective-header"
}' | gcloud secrets versions add testnet-stork-main-oracle-secrets --data-file=-
```

If keyring is enabled, also include:

- `ORACLE_COSMOS_FROM_PASSPHRASE`

### 3) Web2 provider secrets

Web2 provider uses two secrets by default:

- Oracle secrets: `<env>-web2Provider-<name>-oracle-secrets`
- API keys: `<env>-web2Provider-<name>-oracle-api-keys`

Example (`env=testnet`, `name=main`):

```bash
gcloud secrets create testnet-web2Provider-main-oracle-secrets --replication-policy="automatic"
printf '%s' '{
  "ORACLE_COSMOS_PK": "your-private-key"
}' | gcloud secrets versions add testnet-web2Provider-main-oracle-secrets --data-file=-

gcloud secrets create testnet-web2Provider-main-oracle-api-keys --replication-policy="automatic"
printf '%s' '{
  "BINANCE_API_KEY": "your-api-key",
  "SEDA_API_KEY": "your-api-key"
}' | gcloud secrets versions add testnet-web2Provider-main-oracle-api-keys --data-file=-
```

Notes:

- If you set `secret.oracle_secrets_override` or `secret.api_keys_override`, create/update that GSM secret name instead.
- Web2 API key field names depend on your feed templates (the `envsubst` variables used in those template files).
