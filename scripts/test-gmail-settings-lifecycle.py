#!/usr/bin/env python3
"""Exercise the actual provider with encrypted OpenTofu state and synthetic Gmail HTTPS APIs.

Run on Linux with cryptography installed, tofu on PATH, and --provider pointing to
an unpacked locally built provider executable. No Google credentials or network
access are needed; the fixture generates an ephemeral RSA key and rejects all
unknown OAuth subjects, scopes, endpoints and resource mutations.
"""

from __future__ import annotations
import argparse
import base64
import contextlib
import datetime
import http.server
import json
import os
from pathlib import Path
import shutil
import ssl
import subprocess
import tempfile
import threading
import urllib.parse
import uuid

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import padding, rsa
from cryptography.x509.oid import NameOID

SCOPES = {
	"https://www.googleapis.com/auth/gmail.settings.basic",
	"https://www.googleapis.com/auth/gmail.settings.sharing",
}
USERS = {"first@example.test", "second@example.test"}
SETTINGS = {
	"imap": {
		"enabled": False,
		"autoExpunge": True,
		"expungeBehavior": "archive",
		"maxFolderSize": 0,
	},
	"pop": {"accessWindow": "disabled", "disposition": "leaveInInbox"},
	"language": {"displayLanguage": "en"},
	"autoForwarding": {"enabled": False},
	"vacation": {
		"enableAutoReply": False,
		"responseSubject": "",
		"restrictToContacts": False,
		"restrictToDomain": False,
	},
}
FIELDS = {
	"autoForwarding": "auto_forwarding",
	"enabled": "enabled",
	"autoExpunge": "auto_expunge",
	"expungeBehavior": "expunge_behavior",
	"maxFolderSize": "max_folder_size",
	"accessWindow": "access_window",
	"disposition": "disposition",
	"displayLanguage": "display_language",
	"enableAutoReply": "enable_auto_reply",
	"responseSubject": "response_subject",
	"restrictToContacts": "restrict_to_contacts",
	"restrictToDomain": "restrict_to_domain",
}


@contextlib.contextmanager
def proxy(path, handler):
	"""Trust only an ephemeral fixture CA inside the isolated Linux child processes."""
	key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
	name = x509.Name(
		[x509.NameAttribute(NameOID.COMMON_NAME, "Synthetic Gmail fixture")]
	)
	now = datetime.datetime.now(datetime.UTC)
	certificate = (
		x509.CertificateBuilder()
		.subject_name(name)
		.issuer_name(name)
		.public_key(key.public_key())
		.serial_number(x509.random_serial_number())
		.not_valid_before(now - datetime.timedelta(minutes=1))
		.not_valid_after(now + datetime.timedelta(hours=1))
		.add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
		.add_extension(
			x509.SubjectAlternativeName(
				[
					x509.DNSName(host)
					for host in ("oauth2.googleapis.com", "gmail.googleapis.com")
				]
			),
			critical=False,
		)
		.sign(key, hashes.SHA256())
	)
	cert, key_path = path / "ca.pem", path / "tls-key.pem"
	cert.write_bytes(certificate.public_bytes(serialization.Encoding.PEM))
	key_path.write_bytes(
		key.private_bytes(
			serialization.Encoding.PEM,
			serialization.PrivateFormat.PKCS8,
			serialization.NoEncryption(),
		)
	)
	context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
	context.load_cert_chain(cert, key_path)

	class Proxy(http.server.BaseHTTPRequestHandler):
		def log_message(self, *_args):
			pass

		def do_CONNECT(self):
			if self.path not in {
				"oauth2.googleapis.com:443",
				"gmail.googleapis.com:443",
			}:
				self.send_error(403)
				return
			self.send_response(200)
			self.end_headers()
			with context.wrap_socket(self.connection, server_side=True) as secure:
				handler(secure, self.client_address, self.server)
			self.close_connection = True

	server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Proxy)
	server.daemon_threads = True
	thread = threading.Thread(target=server.serve_forever, daemon=True)
	thread.start()
	try:
		yield server, cert
	finally:
		server.shutdown()
		server.server_close()
		thread.join(timeout=5)


def main():
	"""Keep fixture-only imports, remote writes and observed clean plans separately accountable."""
	parser = argparse.ArgumentParser(description=__doc__)
	parser.add_argument("--provider", type=Path, required=True)
	parser.add_argument("--version", default="0.12.2-fork.1")
	args = parser.parse_args()
	with tempfile.TemporaryDirectory(prefix="gmail-settings-native-") as temporary:
		path = Path(temporary)
		signer = rsa.generate_private_key(public_exponent=65537, key_size=2048)
		secret = signer.private_bytes(
			serialization.Encoding.PEM,
			serialization.PrivateFormat.PKCS8,
			serialization.NoEncryption(),
		).decode()
		key_path = path / "credentials.json"
		key_path.write_text(
			json.dumps(
				{
					"type": "service_account",
					"project_id": "example-project",
					"private_key_id": "synthetic-key",
					"private_key": secret,
					"client_email": "planner@example-project.iam.gserviceaccount.com",
					"client_id": "123456789",
					"token_uri": "https://oauth2.googleapis.com/token",
				}
			)
		)
		key_path.chmod(0o600)
		remote = {
			(user, kind): dict(values)
			for user in USERS
			for kind, values in SETTINGS.items()
		}
		tokens, subjects, writes, errors = {}, set(), [], []
		mode = {
			"denied": False,
			"expired": False,
			"read_failure": False,
			"write_denied": False,
			"fail_after_write": False,
			"transient_reads": 0,
		}

		class Gmail(http.server.BaseHTTPRequestHandler):
			def log_message(self, *_args):
				pass

			def reply(self, status, payload):
				data = json.dumps(payload).encode()
				self.send_response(status)
				self.send_header("Content-Type", "application/json")
				self.send_header("Content-Length", str(len(data)))
				self.end_headers()
				self.wfile.write(data)

			def do_POST(self):
				if self.path != "/token":
					errors.append("unexpected POST")
					self.reply(405, {})
					return
				try:
					form = urllib.parse.parse_qs(
						self.rfile.read(int(self.headers["Content-Length"])).decode()
					)
					assert form["grant_type"] == [
						"urn:ietf:params:oauth:grant-type:jwt-bearer"
					]
					header, body, signature = form["assertion"][0].split(".")
					decode = lambda part: base64.urlsafe_b64decode(
						part + "=" * (-len(part) % 4)
					)
					assert json.loads(decode(header))["alg"] == "RS256"
					signer.public_key().verify(
						decode(signature),
						(header + "." + body).encode(),
						padding.PKCS1v15(),
						hashes.SHA256(),
					)
					claims = json.loads(decode(body))
					assert claims["sub"] in USERS
					assert set(claims["scope"].split()) == SCOPES
					assert claims["aud"] == "https://oauth2.googleapis.com/token"
					assert (
						claims["iss"]
						== "planner@example-project.iam.gserviceaccount.com"
					)
					now = datetime.datetime.now(datetime.UTC).timestamp()
					assert claims["iat"] <= now < claims["exp"]
					subjects.add(claims["sub"])
					token = uuid.uuid4().hex
					tokens[token] = claims["sub"]
					self.reply(
						200,
						{
							"access_token": token,
							"token_type": "Bearer",
							"expires_in": 3600,
						},
					)
				except Exception as error:
					errors.append(type(error).__name__)
					self.reply(400, {"error": "rejected synthetic assertion"})

			def identity(self):
				subject = tokens.get(
					self.headers.get("Authorization", "").removeprefix("Bearer ")
				)
				endpoint = urllib.parse.urlsplit(self.path).path
				if not subject or mode["expired"]:
					self.reply(
						401,
						{"error": {"code": 401, "message": "synthetic expired token"}},
					)
					return None
				if mode["denied"]:
					self.reply(403, {"error": {"code": 403, "message": secret}})
					return None
				prefix = "/gmail/v1/users/me/settings/"
				if (
					not endpoint.startswith(prefix)
					or (subject, endpoint.removeprefix(prefix)) not in remote
				):
					errors.append("unexpected settings endpoint")
					self.reply(404, {})
					return None
				return subject, endpoint.removeprefix(prefix)

			def do_GET(self):
				identity = self.identity()
				if identity is None:
					return
				if mode["transient_reads"]:
					mode["transient_reads"] -= 1
					self.reply(
						503,
						{
							"error": {
								"code": 503,
								"message": "synthetic eventual consistency",
							}
						},
					)
					return
				if mode["read_failure"]:
					self.reply(400, {"error": {"code": 400, "message": secret}})
					return
				self.reply(200, remote[identity])

			def do_PUT(self):
				identity = self.identity()
				if identity is None:
					return
				body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
				if mode["write_denied"]:
					self.reply(403, {"error": {"code": 403, "message": secret}})
					return
				if mode["fail_after_write"]:
					mode["read_failure"] = True
				writes.append((identity, body))
				remote[identity] = body
				self.reply(200, body)

			def reject(self):
				errors.append("unexpected mutation")
				self.reply(405, {})

			do_PATCH = do_DELETE = reject

		provider_dir = (
			path
			/ "mirror"
			/ "registry.opentofu.org"
			/ "fishcharlie"
			/ "googleworkspace"
			/ args.version
			/ ("linux_" + {"aarch64": "arm64", "x86_64": "amd64"}[os.uname().machine])
		)
		provider_dir.mkdir(parents=True)
		shutil.copy(
			args.provider,
			provider_dir / ("terraform-provider-googleworkspace_v" + args.version),
		)
		configuration = {
			"terraform": {
				"required_providers": {
					"googleworkspace": {
						"source": "fishcharlie/googleworkspace",
						"version": args.version,
					}
				},
				"encryption": {
					"key_provider": {
						"pbkdf2": {
							"fixture": {
								"passphrase": "synthetic-fixture-only-encryption-key-12345"
							}
						}
					},
					"method": {
						"aes_gcm": {
							"fixture": {"keys": "${key_provider.pbkdf2.fixture}"}
						}
					},
					"state": {"method": "method.aes_gcm.fixture", "enforced": True},
					"plan": {"method": "method.aes_gcm.fixture", "enforced": True},
				},
			},
			"provider": {
				"googleworkspace": {
					"customer_id": "C01234567",
					"oauth_scopes": sorted(SCOPES),
				}
			},
			"resource": {},
			"import": [],
		}
		for endpoint, values in SETTINGS.items():
			kind = FIELDS.get(endpoint, endpoint)
			name = "googleworkspace_gmail_" + kind
			configuration["resource"][name] = {
				"existing": {
					"for_each": {
						str(index): user for index, user in enumerate(sorted(USERS))
					},
					"primary_email": "${each.value}",
					**{
						FIELDS.get(field, field): value
						for field, value in values.items()
					},
					"lifecycle": {"prevent_destroy": True},
				}
			}
			configuration["import"].append(
				{
					"for_each": {
						str(index): user for index, user in enumerate(sorted(USERS))
					},
					"to": name + ".existing[each.key]",
					"id": "${each.value}",
				}
			)
		config = path / "main.tf.json"
		config.write_text(json.dumps(configuration))
		environment = {
			key: value
			for key, value in os.environ.items()
			if not key.startswith(("GOOGLE", "AWS_", "CLOUDSDK", "TF_"))
		}
		environment.update(CHECKPOINT_DISABLE="1")

		def run(*arguments, expected=0):
			print("OpenTofu synthetic lifecycle:", arguments[0], flush=True)
			result = subprocess.run(
				["tofu", "-chdir=" + temporary, *arguments],
				env=environment,
				capture_output=True,
				text=True,
				timeout=180,
			)
			if result.returncode != expected:
				raise AssertionError(
					"OpenTofu "
					+ arguments[0]
					+ " failed: "
					+ result.stderr.replace(secret, "[synthetic secret removed]")
				)
			return result

		run("init", "-input=false", "-plugin-dir=" + str(path / "mirror"))
		with proxy(path, Gmail) as (server, cert):
			environment.update(
				GOOGLEWORKSPACE_CREDENTIALS=str(key_path),
				HTTPS_PROXY=f"http://127.0.0.1:{server.server_port}",
				NO_PROXY="",
				SSL_CERT_FILE=str(cert),
			)
			run("plan", "-input=false", "-out=imports.tfplan")
			plan = json.loads(run("show", "-json", "imports.tfplan").stdout)
			assert len(plan["resource_changes"]) == 10 and all(
				item["change"]["actions"] == ["no-op"]
				and item["change"].get("importing")
				for item in plan["resource_changes"]
			)
			assert subjects == USERS and not writes and not errors
			assert secret not in json.dumps(plan) and str(key_path) not in json.dumps(
				plan
			)
			run("apply", "-input=false", "imports.tfplan")
			run("plan", "-input=false", "-detailed-exitcode")
			assert not writes
			# One reviewed update exercises false/zero serialization, a dynamic language, and vacation format switching.
			configuration["resource"]["googleworkspace_gmail_language"]["existing"][
				"display_language"
			] = "fr"
			vacation = configuration["resource"]["googleworkspace_gmail_vacation"][
				"existing"
			]
			vacation.update(
				response_body="Synthetic <dynamic> reply", response_format="plain_text"
			)
			config.write_text(json.dumps(configuration))
			run("plan", "-input=false", "-out=updates.tfplan")
			run("apply", "-input=false", "updates.tfplan")
			run("plan", "-input=false", "-detailed-exitcode")
			for user in USERS:
				assert remote[user, "language"]["displayLanguage"] == "fr"
				assert (
					remote[user, "vacation"]["responseBodyPlainText"]
					== "Synthetic <dynamic> reply"
				)
				assert (
					"responseBodyHtml" not in remote[user, "vacation"]
					and "endTime" not in remote[user, "vacation"]
				)
			vacation.update(
				response_body="<p>Synthetic HTML</p>", response_format="html"
			)
			config.write_text(json.dumps(configuration))
			run("plan", "-input=false", "-out=html.tfplan")
			run("apply", "-input=false", "html.tfplan")
			run("plan", "-input=false", "-detailed-exitcode")
			vacation.update(response_body="Back to plain", response_format="plain_text")
			config.write_text(json.dumps(configuration))
			run("plan", "-input=false", "-out=plain.tfplan")
			run("apply", "-input=false", "plain.tfplan")
			run("plan", "-input=false", "-detailed-exitcode")
			for user in USERS:
				assert (
					remote[user, "vacation"]["responseBodyPlainText"] == "Back to plain"
					and "responseBodyHtml" not in remote[user, "vacation"]
				)
			for flag in ("denied", "expired", "read_failure"):
				previous = len(writes)
				mode[flag] = True
				failure = run("plan", "-input=false", expected=1)
				mode[flag] = False
				assert (
					secret not in failure.stderr
					and secret not in failure.stdout
					and len(writes) == previous
				)
				run("plan", "-input=false", "-detailed-exitcode")
			# Unknown values produced by another resource must remain valid until apply.
			configuration["resource"]["terraform_data"] = {"reply": {"input": "Dynamic generated response"}}
			vacation.update(enable_auto_reply=True, response_subject="", response_body="${terraform_data.reply.output}")
			config.write_text(json.dumps(configuration))
			run("plan", "-input=false", "-out=unknown-body.tfplan")
			run("apply", "-input=false", "unknown-body.tfplan")
			run("plan", "-input=false", "-detailed-exitcode")
			assert all(remote[user,"vacation"]["responseBodyPlainText"] == "Dynamic generated response" for user in USERS)
			# A denied PUT preserves state and makes no remote changes; a fresh plan can safely resume.
			configuration["resource"]["googleworkspace_gmail_language"]["existing"][
				"display_language"
			] = "de"
			config.write_text(json.dumps(configuration))
			run("plan", "-input=false", "-out=denied-write.tfplan")
			previous = len(writes)
			mode["write_denied"] = True
			failure = run("apply", "-input=false", "denied-write.tfplan", expected=1)
			mode["write_denied"] = False
			assert len(writes) == previous and secret not in failure.stderr
			run("plan", "-input=false", "-out=resumed-write.tfplan")
			# Successful PUT followed by failed refresh retains identity; refresh reconciles without replay.
			mode["fail_after_write"] = True
			run("apply", "-input=false", "resumed-write.tfplan", expected=1)
			mode["fail_after_write"] = mode["read_failure"] = False
			completed = len(writes)
			run("plan", "-input=false", "-detailed-exitcode")
			assert len(writes) == completed
			# A transient read is retried by the existing client; the provider never treats it as absence.
			mode["transient_reads"] = 1
			run("plan", "-input=false", "-detailed-exitcode")
			assert mode["transient_reads"] == 0 and len(writes) == completed
			vacation.update(start_time="1900000000000", end_time="1900000100000")
			config.write_text(json.dumps(configuration))
			run("plan", "-input=false", "-out=dates.tfplan")
			run("apply", "-input=false", "dates.tfplan")
			run("plan", "-input=false", "-detailed-exitcode")
			previous = len(writes)
			vacation["end_time"] = ""
			config.write_text(json.dumps(configuration))
			failure = run("plan", "-input=false", expected=1)
			assert (
				"clearing an existing vacation end_time" in failure.stderr
				and len(writes) == previous
			)
			vacation["end_time"] = "1900000100000"
			configuration["import"][0]["for_each"] = {}
			configuration["resource"]["googleworkspace_gmail_imap"]["existing"][
				"for_each"
			] = {}
			config.write_text(json.dumps(configuration))
			failure = run("plan", "-input=false", expected=1)
			assert "prevent_destroy" in failure.stderr, failure.stderr.replace(
				secret, "[synthetic secret removed]"
			)
			# Singleton deletion forgets only state; no preference-reset endpoint exists.
			configuration["resource"]["googleworkspace_gmail_imap"]["existing"][
				"lifecycle"
			]["prevent_destroy"] = False
			config.write_text(json.dumps(configuration))
			previous = len(writes)
			run("plan", "-input=false", "-out=forget.tfplan")
			run("apply", "-input=false", "forget.tfplan")
			run("plan", "-input=false", "-detailed-exitcode")
			assert len(writes) == previous and all(
				(user, "imap") in remote for user in USERS
			)
			assert (
				"first@example.test" not in (path / "terraform.tfstate").read_text()
				and not errors
			)
			print(
				json.dumps(
					{
						"imports": 10,
						"adoption_writes": 0,
						"fixture_updates": len(writes),
						"mailboxes": len(subjects),
						"encrypted_state": True,
						"clean_plans": True,
						"failure_redaction": True,
					}
				)
			)


if __name__ == "__main__":
	main()
