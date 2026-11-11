# Sapien ex Machina landing page

Live at https://app.sapienexmachina.si. It's a static page in a private S3 bucket, served by CloudFront through Origin Access Control. Suggestions posted on the page go to a small Lambda at `/api/*` via an HTTP API, which writes CloudWatch logs and emails John via SES.

```
site/     index.html, grokbot-to-fierce-man.mp4 (a ping-pong loop: forward then reversed), poster.jpg
lambda/   contact.py (suggestions form handler)
infra/    OpenTofu config and .terraform.lock.hcl. State is NOT stored here.
```

AWS account 457504029722, profile `gonetlogo`. The bucket is `sapienexmachina-landing-457504029722` (us-west-2). The ACM cert is in us-east-1.
The DNS records (A/AAAA alias at the apex plus the cert validation CNAME, SES verification TXT, and SES DKIM CNAMEs) live in the Route 53 zone `app.sapienexmachina.si`
(Z09029301KW3UP21OMYSN), which the GoNetLogo bootstrap stack owns. This stack only adds records to it.

The contact form posts same-origin to `/api/contact`. CloudFront forwards that path to the HTTP API
`sapienexmachina-landing-contact`, which invokes the Lambda of the same name (arm64, Python 3.12, reserved concurrency 2).
CloudFront injects a shared origin secret so a direct call to the API is rejected. The function logs every submission
to `/aws/lambda/sapienexmachina-landing-contact` (30-day retention) and then sends mail from
`suggestions@app.sapienexmachina.si` to `jphelps@iaip.com` with Reply-To set to the submitter if they left an email.

SES lives in us-west-2 (us-east-1 SES is blocked by the account SCP). The account is in the SES sandbox, so John must
click the verification email SES sends to `jphelps@iaip.com` before messages will actually be delivered. Submissions are
still captured in CloudWatch even before that click, or if SES later rejects a send.

## OpenTofu state lives outside this repo

The stack uses **local** state on purpose, and that file is not committed. On the machine that last applied the stack it is:

```
/workspace/landing/infra/terraform.tfstate
```

Backups of the state also stay outside the repo (currently `/workspace/landing/backups/`). The source hero animation
(`src/`) and operational screenshots stay outside the repo as well.

When you run `tofu` from a checkout of `sapienexmachina/infra`, point it at that state file so you do not create a
second state and drift from the live stack:

```bash
cd sapienexmachina/infra
export PATH=~/.local/bin:$PATH
STATE=/workspace/landing/infra/terraform.tfstate
tofu plan  -state="$STATE"
tofu apply -state="$STATE"
cp "$STATE" /workspace/landing/backups/terraform.tfstate.$(date +%Y%m%d-%H%M%S)
```

Alternatively, copy the state into this directory for the duration of the command, then copy it back out and take a
backup. Never commit `terraform.tfstate`, `.terraform/`, `contact.zip`, or backups. A Lambda code change is just a
re-apply (the zip is built from `lambda/contact.py`). Updating the CloudFront distribution (for example a new origin
or cache behavior) takes about 5–15 minutes to deploy.

If the SES email identity for `jphelps@iaip.com` is ever recreated, SES sends another verification email that John must click.

## Update the page

```bash
export PATH=~/.local/bin:$PATH
B=sapienexmachina-landing-457504029722
# distribution_id also comes from: tofu -chdir=sapienexmachina/infra output -state=/workspace/landing/infra/terraform.tfstate -raw distribution_id
DIST=$(cd /workspace/landing/infra && tofu output -raw distribution_id)
cd sapienexmachina/site   # or /workspace/landing/site

# HTML: short cache
aws s3 cp index.html s3://$B/index.html --profile gonetlogo \
  --content-type "text/html; charset=utf-8" --cache-control "public, max-age=300"
# Media: long cache (or sync everything else)
aws s3 sync . s3://$B --profile gonetlogo --exclude index.html \
  --cache-control "public, max-age=31536000"

aws cloudfront create-invalidation --distribution-id $DIST --paths "/*" --profile gonetlogo
```

If you replace the video or poster under the same name, the invalidation is what pushes it out. Browsers may keep the old
file for up to a year, so renaming the file (e.g. `grokbot-to-human-v2.mp4`) is the more reliable way.

Re-encode the hero loop from the source animation, which is kept outside the repo:

```bash
ffmpeg -i /workspace/landing/src/grokbot-to-fierce-man.mp4 -filter_complex "[0:v]reverse[r];[0:v][r]concat=n=2:v=1:a=0,format=yuv420p[v]" \
  -map "[v]" -an -c:v libx264 -preset slow -crf 24 -movflags +faststart sapienexmachina/site/grokbot-to-fierce-man.mp4
```

## Read suggestions

```bash
export PATH=~/.local/bin:$PATH
aws logs tail /aws/lambda/sapienexmachina-landing-contact --since 7d --profile gonetlogo --region us-west-2
```

Look for lines starting with `suggestion submission`. Honeypot hits are logged and dropped; they are not emailed.

## Tear down

The bucket has to be empty first, because it doesn't use force_destroy:

```bash
export PATH=~/.local/bin:$PATH
aws s3 rm s3://sapienexmachina-landing-457504029722 --recursive --profile gonetlogo
cd sapienexmachina/infra
tofu destroy -state=/workspace/landing/infra/terraform.tfstate
```

This removes the distribution, the bucket, the OAC, the cert, the Lambda, the HTTP API, the IAM role, the
CloudWatch log group (and the captured suggestions), the SES domain and email identities, and the landing A/AAAA,
validation, SES TXT, and DKIM records. It leaves the zone and the GoNetLogo records alone. Disabling and deleting the
CloudFront distribution takes about 5–15 minutes. The next time the stack is brought up, John will need to click a new
SES verification email.

Cost: a few cents a month at personal traffic levels (S3 storage of ~3 MB, CloudFront requests and egress, CloudWatch
log ingestion with 30-day retention). Lambda and SES are well inside the free tiers at this volume. Most of the transfer
falls under the CloudFront free tier. Reserved concurrency of 2 caps form-submit compute, not a determined flood of
tiny log events, but those are still cheap.
