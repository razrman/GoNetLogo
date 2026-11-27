#!/bin/bash
# Delete VPC peering connections attached to the gonetlogo VPC that this
# OpenTofu state does not own, and the routes that target them.
#
# openvpn-infra used to create that peering. DeleteVpc then fails with
# DependencyViolation. Connections this state owns are left for
# tofu destroy, which removes their routes and the peering before the VPC.
#
# Run from infra/app after tofu init. The deploy role needs
# ec2:DeleteVpcPeeringConnection and ec2:DeleteRoute, which are in
# infra/bootstrap and are not live until that stack is applied by hand.

set -euo pipefail

vpc_id=$(aws ec2 describe-vpcs \
  --filters Name=tag:Name,Values=gonetlogo \
  --query 'Vpcs[0].VpcId' \
  --output text)

if [ -z "$vpc_id" ] || [ "$vpc_id" = "None" ]; then
  echo "No VPC tagged Name=gonetlogo."
  exit 0
fi

managed=""
if state=$(tofu state pull 2>/dev/null); then
  managed=$(printf '%s\n' "$state" | jq -r '
    [.resources[]?
      | select(.type == "aws_vpc_peering_connection")
      | .instances[]?.attributes.id
      | select(. != null and . != "")] | .[]')
fi

live=$(aws ec2 describe-vpc-peering-connections \
  --filters Name=status-code,Values=initiating-request,pending-acceptance,provisioning,active \
  --output json \
  | jq -r --arg vpc "$vpc_id" '
      .VpcPeeringConnections[]?
      | select(.RequesterVpcInfo.VpcId == $vpc or .AccepterVpcInfo.VpcId == $vpc)
      | .VpcPeeringConnectionId')

foreign=()
while IFS= read -r id; do
  [ -z "$id" ] && continue
  if printf '%s\n' "$managed" | grep -qx "$id"; then
    echo "Peering ${id} is in this state. OpenTofu will destroy it."
    continue
  fi
  foreign+=("$id")
done <<< "$live"

if [ "${#foreign[@]}" -eq 0 ]; then
  echo "No foreign peering on ${vpc_id}."
  exit 0
fi

echo "Foreign peering on ${vpc_id} would block VPC delete:"
printf '  %s\n' "${foreign[@]}"

for pcx in "${foreign[@]}"; do
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    rt=${line%% *}
    cidr=${line#* }
    echo "Deleting route ${cidr} on ${rt}"
    aws ec2 delete-route --route-table-id "$rt" --destination-cidr-block "$cidr"
  done < <(aws ec2 describe-route-tables \
    --filters "Name=route.vpc-peering-connection-id,Values=${pcx}" \
    --output json \
    | jq -r --arg pcx "$pcx" '
        .RouteTables[]?
        | .RouteTableId as $id
        | .Routes[]?
        | select(.VpcPeeringConnectionId == $pcx and .DestinationCidrBlock != null)
        | "\($id) \(.DestinationCidrBlock)"')

  echo "Deleting peering ${pcx}"
  if ! err=$(aws ec2 delete-vpc-peering-connection --vpc-peering-connection-id "$pcx" 2>&1); then
    echo "Could not delete ${pcx}: ${err}" >&2
    echo "Apply infra/bootstrap so gonetlogo-github-deploy has ec2:DeleteVpcPeeringConnection and ec2:DeleteRoute. A peering left on ${vpc_id} makes Infra down fail with DependencyViolation on the VPC." >&2
    exit 1
  fi
done
