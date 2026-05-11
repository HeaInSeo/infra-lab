#!/usr/bin/env bash
set -euo pipefail

echo "== gateways =="
kubectl get gateway -A -o wide

echo
echo "== gateway yaml =="
kubectl get gateway -A -o yaml

echo
echo "== httproutes =="
kubectl get httproute -A -o yaml

echo
echo "== grpcroutes =="
kubectl get grpcroute -A -o yaml

echo
echo "== envoy configs =="
kubectl get ciliumenvoyconfig -A
