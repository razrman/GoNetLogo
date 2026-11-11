"""Landing-page suggestions form. Logs every submission, then emails John via SES."""

from __future__ import annotations

import base64
import hmac
import json
import logging
import os
import re
import boto3
from botocore.exceptions import ClientError

logger = logging.getLogger()
logger.setLevel(logging.INFO)

ses = boto3.client("ses", region_name=os.environ.get("SES_REGION", "us-west-2"))

FROM_ADDR = os.environ["FROM_ADDR"]
TO_ADDR = os.environ["TO_ADDR"]
ORIGIN_SECRET = os.environ.get("ORIGIN_SECRET", "")

MAX_NAME = 200
MAX_EMAIL = 254
MAX_MESSAGE = 4000
MAX_BODY = 20_000
EMAIL_RE = re.compile(r"^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$")


def _response(status: int, body: dict) -> dict:
    return {
        "statusCode": status,
        "headers": {
            "content-type": "application/json; charset=utf-8",
            "cache-control": "no-store",
        },
        "body": json.dumps(body),
    }


def _header(event: dict, name: str) -> str:
    headers = event.get("headers") or {}
    want = name.lower()
    for key, value in headers.items():
        if str(key).lower() == want:
            return value or ""
    return ""


def _parse_body(event: dict) -> dict:
    raw = event.get("body") or ""
    if event.get("isBase64Encoded"):
        raw = base64.b64decode(raw).decode("utf-8", errors="replace")
    if len(raw) > MAX_BODY:
        raise ValueError("too-large")
    if not raw.strip():
        return {}
    data = json.loads(raw)
    if not isinstance(data, dict):
        raise ValueError("not-object")
    return data


def _field(data: dict, key: str, limit: int) -> str:
    value = data.get(key, "")
    if value is None:
        return ""
    if not isinstance(value, str):
        value = str(value)
    return value.strip()[: limit + 1]


def handler(event, _context):
    http = (event.get("requestContext") or {}).get("http") or {}
    method = http.get("method") or event.get("httpMethod") or ""
    path = (event.get("rawPath") or event.get("path") or "").rstrip("/")
    source_ip = http.get("sourceIp") or "unknown"

    if method == "OPTIONS":
        return _response(204, {})
    if method != "POST" or path != "/api/contact":
        return _response(404, {"ok": False, "error": "Not found."})

    # Only accept requests that came through CloudFront (custom origin header).
    provided = _header(event, "x-origin-secret")
    if not ORIGIN_SECRET or not hmac.compare_digest(provided, ORIGIN_SECRET):
        logger.info("rejected request without a valid origin secret ip=%s", source_ip)
        return _response(403, {"ok": False, "error": "Forbidden."})

    try:
        data = _parse_body(event)
    except ValueError as exc:
        if str(exc) == "too-large":
            return _response(400, {"ok": False, "error": "Message is too long."})
        return _response(400, {"ok": False, "error": "Please send the form as JSON."})
    except json.JSONDecodeError:
        return _response(400, {"ok": False, "error": "Please send the form as JSON."})

    # Honeypot: pretend success so bots don't retry, but drop the message.
    if _field(data, "website", 200) or _field(data, "honeypot", 200):
        logger.info("honeypot hit; dropping ip=%s", source_ip)
        return _response(200, {"ok": True})

    name = _field(data, "name", MAX_NAME)
    email = _field(data, "email", MAX_EMAIL)
    message = _field(data, "message", MAX_MESSAGE)

    if not message:
        return _response(400, {"ok": False, "error": "Please enter a message."})
    if len(name) > MAX_NAME:
        return _response(400, {"ok": False, "error": "Name is too long."})
    if len(email) > MAX_EMAIL:
        return _response(400, {"ok": False, "error": "Email is too long."})
    if email and not EMAIL_RE.match(email):
        return _response(400, {"ok": False, "error": "Please enter a valid email, or leave it blank."})
    if len(message) > MAX_MESSAGE:
        return _response(400, {"ok": False, "error": "Message is too long."})

    # Persist first so a sandbox / unverified SES failure cannot lose the note.
    logger.info(
        "suggestion submission name=%r email=%r ip=%s message=%r",
        name,
        email,
        source_ip,
        message,
    )

    lines = [
        "A suggestion was submitted on https://app.sapienexmachina.si",
        "",
        f"Name: {name or '(not given)'}",
        f"Email: {email or '(not given)'}",
        f"IP: {source_ip}",
        "",
        "Message:",
        message,
    ]
    kwargs = {
        "Source": FROM_ADDR,
        "Destination": {"ToAddresses": [TO_ADDR]},
        "Message": {
            "Subject": {
                "Data": "Suggestion from the Sapien ex Machina landing page",
                "Charset": "UTF-8",
            },
            "Body": {"Text": {"Data": "\n".join(lines), "Charset": "UTF-8"}},
        },
    }
    if email:
        kwargs["ReplyToAddresses"] = [email]

    try:
        ses.send_email(**kwargs)
        logger.info("SES send ok to=%s from=%s", TO_ADDR, FROM_ADDR)
    except ClientError:
        logger.exception("SES send failed; submission is in the log above")
    except Exception:
        logger.exception("SES send failed; submission is in the log above")

    return _response(200, {"ok": True})
