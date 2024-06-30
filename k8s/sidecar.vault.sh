#!/usr/bin/env bash
vault policy write flags-gg-orchestrator-sidecar-policy - <<EOF
  path "kv/data/flags-gg/orchestrator" {
    capabilities = ["read", "list"]
  }
EOF

vault write auth/kubernetes/role/flags-gg-vault-orchestrator-sidecar \
  bound_service_account_names=vault-orchestrator-sidecar \
  bound_service_account_namespaces=flags-gg \
  policies=flags-gg-orchestrator-sidecar-policy \
  ttl=24h
