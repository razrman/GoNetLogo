# AWS

OpenTofu layout for the web server. Two stacks:

| Stack | State | Survives teardown |
| --- | --- | --- |
| `infra/bootstrap` | Local file on the machine that applied it | Yes. State bucket, GitHub OIDC provider, deploy role, ECR repository, Route 53 zone, ACM certificate, budget, image-tag parameter |
| `infra/app` | S3 bucket from bootstrap, locked with an S3 lockfile (`use_lockfile`, no DynamoDB) | No. VPC, subnets, security groups, load balancer, ECS service, log group, and the alias record |

Account `457504029722`, region `us-west-2`. The provider refuses any other account. Bootstrap is applied by hand with an administrator key. After that, GitHub Actions assumes `gonetlogo-github-deploy` through OIDC. There are no long-lived AWS keys in the repository.

The task is Fargate on Graviton, 0.5 vCPU and 1 GB, one task, `linux/arm64`. The image is also pushed for `linux/amd64`. Graviton is the cheaper of the two sizes the Dockerfile already builds. Sessions are in memory in that one process, so the service stays at one task. A second task would split clients. The container entrypoint already sets `-addr 0.0.0.0:8080` and `-log-format json`. The task sets `GONETLOGO_PRODUCTION=1`, `GONETLOGO_MAX_SESSIONS=32`, `GONETLOGO_ALLOWED_ORIGINS=https://gonetlogo.sapienexmachina.si`, and `GONETLOGO_SHUTDOWN_TIMEOUT=20s`. Fargate's stop timeout is 30 seconds, and the load balancer waits 30 seconds before the task is signaled. The load balancer idle timeout is 300 seconds. The server pings websockets every 10 seconds and the page heartbeats every 15 seconds. `GET /healthz` is the target-group check. The image has no shell, so there is no in-container probe. `/tmp` is the task's 20 GB ephemeral disk.

The app README's note about 32 sessions on a 2 GB task is a larger size than this one. With 512 CPU units, memory can be raised to 2048, 3072, or 4096 in `infra/app/variables.tf` without changing the CPU.

## Cost

Prices checked against the public AWS list for us-west-2 on 2026-11-11. A month is taken as 730 hours. This is a list-price sketch, not a bill.

**Torn down** (bootstrap only): about **$0.50 to $1 per month**.

| Piece | About |
| --- | --- |
| Route 53 hosted zone | $0.50 |
| ECR storage for a handful of images | a few cents |
| S3 state, ACM, IAM, SSM, the budget (the first two budgets are free) | a few cents |

**Running** (app stack up, one task): about **$45 to $55 per month**, plus the zone.

| Piece | About |
| --- | --- |
| Fargate 0.5 vCPU and 1 GB, ARM, always on | $14 |
| Application load balancer hourly charge, plus a small amount of capacity at low traffic | $18 to $22 |
| Public IPv4: two on the load balancer and one on the task, at $0.005 per hour | $11 |
| Logs, data transfer under the free 100 GB, the zone | $1 to $2 |

There is no NAT gateway. A NAT gateway would be about $32 per month per availability zone before any data. Tearing the app stack down stops the Fargate, load balancer, public IPv4, and log charges. Bringing it back does not change the GoDaddy records.

A monthly AWS budget of $20 (before credits) emails when actual or forecasted usage crosses that line. Left running, this stack passes $20 well before the end of the month. Torn down, it stays under it.

## Free-plan accounts

`ecs:*`, `elasticloadbalancing:*`, `ecr:*`, `ec2:*`, `route53:*`, `acm:*`, `logs:*`, `s3:*`, `ssm:*`, and `budgets:*` are on the published allow list for Sign up for AWS (new). Fargate is an ECS launch type, not its own service prefix.

Two limits on that sign-up path are easy to hit:

- The free-plan and paid-plan service control policies deny `iam:*Provider*` until advanced AWS features are turned on. Bootstrap creates a GitHub OIDC provider, and that call fails until then. ECS, the load balancer, ECR, Route 53, and ACM do not need that switch.
- As a project nears a spend limit, AWS denies `elasticloadbalancing:CreateLoadBalancer`, `ecs:CreateService`, and `ecr:CreateRepository`. A limit at or below the running cost makes the next apply fail with `AccessDenied`. The $20 budget in this stack is an email. It is not that hard cap. Raise the account spend limit for any month the stack stays up, or tear the app stack down first.

The free plan is a credit balance (up to $200) for up to six months, then the account closes unless it is moved to a paid plan. Credits do not make this stack free. The budget is configured with credits excluded, so the mail still arrives while credits are paying the bill. The first budget email is a confirmation link. Alerts do not send until that link is opened.

A classic account (Sign up for AWS advanced, or an account opened before that program) does not have those service control policies. The prices above still apply.

## One-time bootstrap

Install OpenTofu `1.13.9` (the pin is `infra/.opentofu-version`). From a shell whose AWS credentials are an administrator in account `457504029722`:

```bash
cd infra/bootstrap
tofu init
tofu apply
```

Keep `terraform.tfstate` from that directory somewhere private. It is gitignored. It is the only record of the bucket, the role, the zone, and the certificate. A later apply in this directory does not reset the image tag; that value is ignored after the first write.

`tofu output -raw deploy_role_arn` is the role ARN.

`tofu output -json name_servers` is four Route 53 name servers.

The apply also requests the certificate and writes its validation CNAME into the new hosted zone. That record is not copied to GoDaddy.

### GoDaddy, once

In the `sapienexmachina.si` zone, add NS records for the host `gonetlogo`, one for each name server from the output. Leave the apex (`@`) and `www` records as they are. They keep serving the existing site. If GoDaddy already has an A or CNAME on `gonetlogo`, remove that record first. A name cannot be delegated and also be a CNAME.

Delegation is the only GoDaddy edit. After it propagates, ACM marks the certificate Issued on its own. Check it with:

```bash
aws acm describe-certificate \
  --certificate-arn "$(tofu output -raw certificate_arn)" \
  --region us-west-2 \
  --query 'Certificate.Status' \
  --output text
```

To have OpenTofu wait (up to 45 minutes) instead of polling:

```bash
tofu apply -var='wait_for_certificate=true'
```

The default of `wait_for_certificate` stays `false`, so a later bootstrap apply does not sit on DNS. The validation CNAME remains in the zone either way. ACM needs that record to stay for renewal. Destroying the app stack does not remove it.

Accept the budget confirmation mail sent to `budget_email` (`jphelps@iaip.com` unless you change `infra/bootstrap/variables.tf`).

### GitHub

Repository **variable** (Settings → Secrets and variables → Actions → Variables), not a secret:

| Name | Value |
| --- | --- |
| `AWS_DEPLOY_ROLE_ARN` | `deploy_role_arn` from the bootstrap output |

Environment name: `production`. The role trusts only this subject:

`repo:razrman@1226040/GoNetLogo@1423386498:environment:production`

GoNetLogo was created on 2026-10-17. Repositories created after 2026-07-15 put the owner id and the repository id in the OIDC subject. The older string `repo:razrman/GoNetLogo:environment:production` does not match tokens this repository issues. `github_subject` in `infra/bootstrap/variables.tf` is the one line to change if a failed assume-role shows a different subject.

On the environment:

- Add a required reviewer.
- Allow deployment from the branches you open pull requests from, from `main`, and from tags matching `[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9]`. "All branches" plus that tag pattern is the setting that lets pull-request plans, Infra up, Infra down, and tag deploys all assume the role. The required reviewer is the gate. A tag deploy or an Infra down run that fails immediately with an environment protection error is missing the tag or the branch on that list.

No other repository variables or secrets are required.

## Bring the app up

After the certificate status is `ISSUED` and the GitHub variable exists:

Actions → **Infra up** → Run workflow.

Or locally, with the deploy role or an administrator:

```bash
cd infra/app
tofu init
tofu apply
```

That creates the network, the load balancer, the ECS service, the log group, and an alias A record at `gonetlogo.sapienexmachina.si` aimed at the load balancer. Port 80 redirects to 443. The load balancer has deletion protection off.

Until an image tag has been published, the service is created at desired count 0, because the SSM parameter still says `bootstrap` and that image does not exist. The first tag deploy starts the task. After that, the parameter holds the real tag, and a later Infra up starts the last image without another deploy.

The alias is updated in the same apply that creates the load balancer. The load balancer's own DNS name changes every time the app stack is created. GoDaddy does not. Clients use `https://gonetlogo.sapienexmachina.si`, and only from `allowed_cidrs` (default `68.118.144.13/32`) on ports 80 and 443.

Infra up fails with a missing-certificate error if it runs before ACM has issued the certificate. Wait for `ISSUED` and run it again.

## Deploy

Push a tag `YYYYMMDD-XX`, for example `20261111-03`. The Deploy workflow checks out that tag, assumes the role, and stops immediately if the ECS service is not `ACTIVE`, with the message to run Infra up first. Otherwise it builds `linux/amd64` and `linux/arm64`, pushes the tag and the commit sha to ECR, writes the tag to SSM, registers a new task definition revision, and waits until the service is stable on that revision. A rolled-back deployment fails the job.

Re-run a tag from the Actions tab with the Deploy workflow's `image_tag` input. That checks out the tag you name, not the branch selected in the UI.

A deploy drops in-memory sessions. The circuit breaker rolls a bad image back to the previous revision.

ECR keeps the last 10 images and scans on push (basic scanning, not Inspector). Tags are mutable so a failed deploy of the same tag can be pushed again.

## Tear the app down

Actions → **Infra down** → type `destroy` in the confirm box.

Or:

```bash
cd infra/app
tofu destroy
```

One destroy removes the VPC, subnets, security groups, load balancer, target group, listeners, ECS cluster and service, log group, and the alias record. Destroy waits up to 20 minutes for Fargate network interfaces to release. There is no bucket to empty and no deletion protection to turn off. CloudWatch logs for the service are deleted with the log group.

Left in place: the state bucket and the app state file, the OIDC provider, the deploy role, the hosted zone and its NS and certificate-validation records, the certificate, the ECR images, the budget, and the image tag. The hostname stops resolving until the next Infra up, which writes a new alias at the new load balancer. GoDaddy is not edited.

A tag pushed while the stack is down fails the Deploy workflow before the image build.

## Change who can connect

Edit `allowed_cidrs` in `infra/app/variables.tf`. The default is `68.118.144.13/32`. Merge that change, or apply locally, then run Infra up. The security group updates in place. Tasks are not replaced. An empty list is rejected.

CPU, memory, the session cap, and the allowed origin are the other defaults in that file. Changing them and running Infra up registers a new task definition and rolls the service. The next image deploy copies whatever task definition is running, so apply infra changes before you rely on the next tag to carry them.

## Full teardown

Destroy the app stack first. Then delete every version in the state bucket, then destroy bootstrap. Bootstrap destroy removes the zone (the GoDaddy NS records then point nowhere; delete them), the certificate, the images, the role, and the bucket.

```bash
cd infra/app
tofu destroy

BUCKET=gonetlogo-tfstate-457504029722
aws s3api list-object-versions --bucket "$BUCKET" --output json \
  | jq -r '.Versions[]?, .DeleteMarkers[]? | [.Key, .VersionId] | @tsv' \
  | while IFS="$(printf '\t')" read -r key version; do
      aws s3api delete-object --bucket "$BUCKET" --key "$key" --version-id "$version"
    done

cd ../bootstrap
tofu destroy
```

Remove the four NS records for `gonetlogo` at GoDaddy after the zone is gone.

## Pull requests

A pull request that touches `infra/` or these workflows runs `tofu fmt -check` and `tofu validate` on both stacks. `tofu plan` for the app stack runs only when the pull request is from this repository and `AWS_DEPLOY_ROLE_ARN` is set. Forks, and this repository before the variable exists, skip the plan instead of failing. The plan job uses the production environment, so a required reviewer approves it before AWS is called. If you mark checks as required, require fmt and validate. The plan job is skipped until the variable exists, and a skipped required check can block a merge.

Bootstrap is not planned in GitHub. An empty local state on the runner would ask to create the bucket again.

## What the role can do

The deploy role's trust policy is `sts:AssumeRoleWithWebIdentity` for the one subject above and audience `sts.amazonaws.com`. Its policies cover the state bucket, pushing to the `gonetlogo` ECR repository, the `gonetlogo` ECS cluster and service, registering task definitions, passing `gonetlogo-ecs-execution` to ECS, the image-tag parameter, the VPC and load balancer APIs in us-west-2, the log group, reading the certificate, and changing records in this hosted zone. It cannot delete the hosted zone, the certificate, or the ECR repository, and it cannot edit its own trust policy. The task has no task role. The server does not call AWS.

Registering a task definition is granted on `*`. ECS does not accept a task-definition ARN for that action. Only `ecs:UpdateService` on `gonetlogo` can point a service at a revision.

## Troubleshooting

- **AssumeRole failed.** The workflow job must set `environment: production`. Compare the subject in the error with `github_subject`. Advanced features must be on before the OIDC provider can be created.
- **AccessDenied on CreateLoadBalancer or CreateService.** The account spend limit is in the way. Raise it, or you are near it.
- **AccessDenied on CreateOpenIDConnectProvider.** Turn on advanced AWS features and apply bootstrap again. Resources that were created before the failure are kept.
- **App apply cannot find a certificate.** The certificate is not `ISSUED` yet. Finish the NS delegation and wait, or run bootstrap with `-var='wait_for_certificate=true'`.
- **Deploy says the app stack is not up.** Run Infra up. A destroyed service is not `ACTIVE`.
- **Tasks fail to pull.** The task needs its public address and outbound 443. The image architecture must include the task architecture. The deploy pushes both.
- **Unhealthy targets.** The task security group allows 8080 only from the load balancer. The check is `GET /healthz` and expects 200. The log group is `/ecs/gonetlogo`.
