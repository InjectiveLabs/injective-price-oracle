# Price Oracle Helm Chart

This chart (`price-oracle`) deploys oracle instances grouped by provider type.

Each enabled instance renders:

- 1 `Deployment`
- 1 feed `ConfigMap`
- 1 `ExternalSecret` for oracle secrets
- +1 additional `ExternalSecret` for API keys when provider type is `web2Provider`

`ExternalSecret` resources reference the `ClusterSecretStore` named `gcp-secret-store`.

## Values Structure

Define instances under `oracle.<providerType>.<instanceName>`.

- Provider type is inferred from the parent key (`chainlink`, `stork`, `web2Provider`).
- `name` is optional; if omitted, the chart uses the map key as the instance name.
- `env` and `cluster_name` are top-level values shared by all instances.
- `env` (for example `testnet` / `mainnet`) is used as prefix for default GSM secret names.

Example:

```yaml
env: testnet
cluster_name: testnet

oracle:
  chainlink:
    main:
      enabled: true
      version: v1.2.8
      env:
        oracleEnv: testnet
        logLevel: info
        chain:
          chainId: injective-888
          grpcEndpoint: testnet.sentry.chain.grpc.injective.network:443
          tmEndpoint: https://testnet.sentry.tm.injective.network:443
        chainlink:
          websocketUrl: wss://ws.testnet-dataengine.chain.link
      secret:
        refreshInterval: 1h
        oracle_secrets_override: ""
      feedConfigFiles:
        - name: chainlink_btcusd.toml
          content: |
            provider = "chainlink"
            ticker = "BTC/USD"
            oracleType = "Chainlink"
```

## Common Instance Fields

Typical required fields per instance:

- `enabled`
- `version`
- `env.chain.grpcEndpoint`
- `env.chain.tmEndpoint`
- `secret.refreshInterval`
- `feedConfigFiles` (list of TOML files)

Optional but commonly used:

- `tolerations`
- `env.chain.grpcStreamEndpoint`
- `env.keyring.*`
- `secret.oracle_secrets_override`
- `secret.api_keys_override` (only for `web2Provider`)

## Provider Specific Notes

### Chainlink

- Set `env.chainlink.websocketUrl`.
- Oracle secret must include:
  - `ORACLE_COSMOS_PK`
  - `CHAINLINK_API_KEY`
  - `CHAINLINK_API_SECRET`

### Stork

- Set `env.stork.websocketUrl`.
- Set `env.stork.websocketSubscribeMessage`.
- Oracle secret must include:
  - `ORACLE_COSMOS_PK`
  - `STORK_WEBSOCKET_HEADER`

### Web2 Provider

- Feed files are template files rendered at runtime (`envsubst`) by an init container.
- API key variables are loaded from the generated `*-oracle-api-keys` Kubernetes secret.
- Two ExternalSecrets are created:
  - oracle secrets (`*-oracle-secrets`)
  - API keys (`*-oracle-api-keys`)

## Default Secret Names

Unless overridden, the chart reads these GSM secrets:

- Oracle secrets: `<env>-<type>-<name>-oracle-secrets`
- Web2 API keys: `<env>-<type>-<name>-oracle-api-keys`

Override fields:

- `secret.oracle_secrets_override`
- `secret.api_keys_override` (web2 provider only)

## Keyring Support

If `env.keyring.enabled: true`, also provide:

- `ORACLE_COSMOS_FROM_PASSPHRASE`

and configure `env.keyring` fields such as:

- `keyring`
- `keyringDir`
- `keyringApp`
- `cosmosFrom`
- `useLedger`

## Render Chart

```bash
helm template price-oracle ./helm -f ./helm/values.testnet.yaml
```

## Google Secret Manager Examples

`ExternalSecret` uses `dataFrom.extract`, so each GSM secret value should be a JSON object.

Chainlink example:

```bash
gcloud secrets create testnet-chainlink-main-oracle-secrets --replication-policy="automatic"
printf '%s' '{
  "ORACLE_COSMOS_PK": "your-private-key",
  "CHAINLINK_API_KEY": "your-chainlink-api-key",
  "CHAINLINK_API_SECRET": "your-chainlink-api-secret"
}' | gcloud secrets versions add testnet-chainlink-main-oracle-secrets --data-file=-
```

Stork example:

```bash
gcloud secrets create testnet-stork-main-oracle-secrets --replication-policy="automatic"
printf '%s' '{
  "ORACLE_COSMOS_PK": "your-private-key",
  "STORK_WEBSOCKET_HEADER": "secret-injective-header"
}' | gcloud secrets versions add testnet-stork-main-oracle-secrets --data-file=-
```

Web2 provider example:

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
