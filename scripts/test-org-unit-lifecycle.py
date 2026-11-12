#!/usr/bin/env python3
"""Exercise real provider OU imports, updates and deletion against synthetic HTTPS only."""
from __future__ import annotations

import argparse
import importlib.util
import http.server
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import urllib.parse

# Reuse the bounded TLS proxy; no workstation trust store or live key is involved.
spec = importlib.util.spec_from_file_location('google_fixture', Path(__file__).with_name('test-gmail-settings-lifecycle.py'))
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)


def main():
	"""Keep fixture writes distinct from import-only production adoption evidence."""
	parser = argparse.ArgumentParser(description=__doc__)
	parser.add_argument('--provider', required=True, type=Path)
	parser.add_argument('--version', default='0.12.3-fork.1')
	args = parser.parse_args()
	with tempfile.TemporaryDirectory(prefix='org-unit-native-') as temporary:
		path = Path(temporary)
		remote = {
			'id:root': {'orgUnitId': 'id:root', 'name': 'Example', 'description': 'Root', 'orgUnitPath': '/'},
			'id:one': {'orgUnitId': 'id:one', 'name': 'First Ω', 'description': 'Keep', 'orgUnitPath': '/First Ω', 'parentOrgUnitId': 'id:root', 'parentOrgUnitPath': '/'},
			'id:two': {'orgUnitId': 'id:two', 'name': 'Second', 'description': 'Clear this', 'orgUnitPath': '/First Ω/Second', 'parentOrgUnitId': 'id:one', 'parentOrgUnitPath': '/First Ω'},
		}
		mode = {'denied': False, 'pending': {'id:two'}}
		writes = []
		secret = 'synthetic-token-must-not-echo'
		class Google(http.server.BaseHTTPRequestHandler):
			def log_message(self, *_args): pass
			def reply(self, code, payload):
				body = json.dumps(payload).encode()
				self.send_response(code)
				self.send_header('Content-Type', 'application/json')
				self.send_header('Content-Length', str(len(body)))
				self.end_headers()
				self.wfile.write(body)
			def handle_api(self):
				prefix = '/admin/directory/v1/customer/C01234567/orgunits'
				resource = urllib.parse.unquote(urllib.parse.urlsplit(self.path).path)
				if not resource.startswith(prefix): return self.reply(400, {})
				if self.headers.get('Authorization') != 'Bearer ' + secret: return self.reply(401, {'error': {'code': 401, 'message': 'expired ' + secret}})
				if mode['denied']: return self.reply(403, {'error': {'code': 403, 'message': 'denied ' + secret}})
				identifier = resource.removeprefix(prefix).lstrip('/')
				if self.command == 'GET' and identifier not in remote:
					identifier = next((key for key, row in remote.items() if row['orgUnitPath'].lstrip('/') == identifier), identifier)
				if self.command == 'GET':
					if identifier in mode['pending']:
						mode['pending'].remove(identifier)
						return self.reply(404, {'error': {'code': 404}})
					if identifier not in remote: return self.reply(404, {'error': {'code': 404}})
					if self.headers.get('If-None-Match') == remote[identifier].get('etag', 'one'): return self.reply(304, {})
					return self.reply(200, remote[identifier] | {'etag': remote[identifier].get('etag', 'one')})
				if self.command == 'DELETE':
					assert identifier != 'id:root'
					writes.append(('DELETE', identifier))
					remote.pop(identifier, None)
					return self.reply(204, {})
				body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
				assert self.command in {'POST', 'PUT'}
				assert not ('parentOrgUnitId' in body and 'parentOrgUnitPath' in body)
				if self.command == 'POST':
					identifier = 'id:created'
					assert identifier not in remote
					remote[identifier] = {'orgUnitId': identifier}
					mode['pending'].add(identifier)
				assert identifier != 'id:root'
				writes.append((self.command, identifier, body))
				remote[identifier].update(body)
				if 'parentOrgUnitPath' in body:
					remote[identifier]['parentOrgUnitId'] = next(key for key, value in remote.items() if value['orgUnitPath'] == body['parentOrgUnitPath'])
				parent = remote[remote[identifier]['parentOrgUnitId']]
				remote[identifier]['parentOrgUnitPath'] = parent['orgUnitPath']
				remote[identifier]['orgUnitPath'] = parent['orgUnitPath'].rstrip('/') + '/' + remote[identifier]['name']
				remote[identifier]['etag'] = 'after-' + str(len(writes))
				return self.reply(200, remote[identifier])
			do_GET = do_POST = do_PUT = do_DELETE = handle_api

		platform = 'linux_' + {'aarch64': 'arm64', 'x86_64': 'amd64'}[os.uname().machine]
		provider = path / 'mirror/registry.opentofu.org/fishcharlie/googleworkspace' / args.version / platform
		provider.mkdir(parents=True)
		shutil.copy(args.provider, provider / ('terraform-provider-googleworkspace_v' + args.version))
		config = {
			'terraform': {'required_providers': {'googleworkspace': {'source': 'fishcharlie/googleworkspace', 'version': args.version}}, 'encryption': {'key_provider': {'pbkdf2': {'fixture': {'passphrase': 'fixture-only-encryption-passphrase-12345'}}}, 'method': {'aes_gcm': {'fixture': {'keys': '${key_provider.pbkdf2.fixture}'}}}, 'state': {'method': 'method.aes_gcm.fixture', 'enforced': True}, 'plan': {'method': 'method.aes_gcm.fixture', 'enforced': True}}},
			'provider': {'googleworkspace': {'customer_id': 'C01234567'}},
			'data': {'googleworkspace_org_unit': {'root': {'org_unit_id': 'id:root'}, 'by_path': {'org_unit_path': '/First Ω'}}},
			'resource': {'googleworkspace_org_unit': {'existing': {'for_each': {key: {'name': row['name'], 'description': row['description'], 'parent_id': row['parentOrgUnitId']} for key, row in remote.items() if key != 'id:root'}, 'name': '${each.value.name}', 'description': '${each.value.description}', 'parent_org_unit_id': '${each.value.parent_id}', 'block_inheritance': False, 'lifecycle': {'prevent_destroy': True}}}},
			'import': [{'for_each': {key: key for key in remote if key != 'id:root'}, 'to': 'googleworkspace_org_unit.existing[each.key]', 'id': '${each.value}'}],
		}
		config_path = path / 'main.tf.json'
		def save(): config_path.write_text(json.dumps(config))
		save()
		env = {key: value for key, value in os.environ.items() if not key.startswith(('GOOGLE', 'AWS_', 'CLOUDSDK', 'TF_'))}
		env['CHECKPOINT_DISABLE'] = '1'
		def run(*arguments, expected=0):
			print('OU native lifecycle:', arguments[0], flush=True)
			result = subprocess.run(['tofu', '-chdir=' + temporary, *arguments], env=env, capture_output=True, text=True, timeout=180)
			assert result.returncode == expected, (arguments, result.stdout.replace(secret, '[removed]'), result.stderr.replace(secret, '[removed]'))
			return result
		run('init', '-input=false', '-plugin-dir=' + str(path / 'mirror'))
		with fixture.proxy(path, Google, hosts=('admin.googleapis.com',)) as (server, cert):
			env.update(GOOGLE_OAUTH_ACCESS_TOKEN=secret, HTTPS_PROXY='http://127.0.0.1:' + str(server.server_port), NO_PROXY='', SSL_CERT_FILE=str(cert))
			run('plan', '-input=false', '-out=imports.tfplan')
			plan = json.loads(run('show', '-json', 'imports.tfplan').stdout)
			assert len(plan['resource_changes']) == 2 and all(row['change']['actions'] == ['no-op'] and row['change'].get('importing') for row in plan['resource_changes'])
			assert not writes and not mode['pending']
			run('apply', '-input=false', 'imports.tfplan')
			run('plan', '-input=false', '-detailed-exitcode')
			assert secret not in (path / 'terraform.tfstate').read_text()
			rows = config['resource']['googleworkspace_org_unit']['existing']['for_each']
			rows['id:two'].update(name='Moved β', description='', parent_id='id:root')
			save()
			run('plan', '-input=false', '-out=update.tfplan')
			update = json.loads(run('show', '-json', 'update.tfplan').stdout)
			assert [row['change']['actions'] for row in update['resource_changes']].count(['update']) == 1
			run('apply', '-input=false', 'update.tfplan')
			run('plan', '-input=false', '-detailed-exitcode')
			assert writes[0][2]['parentOrgUnitId'] == 'id:root' and writes[0][2]['description'] == ''
			# Critical adopted OUs cannot disappear merely because a declaration is removed.
			saved = rows.pop('id:two')
			config['import'][0]['for_each'].pop('id:two')
			save()
			assert 'prevent_destroy' in run('plan', '-input=false', expected=1).stderr
			rows['id:two'] = saved
			config['import'][0]['for_each']['id:two'] = 'id:two'
			config['resource']['googleworkspace_org_unit']['temporary'] = {'name': 'Disposable', 'parent_org_unit_path': '/', 'description': ''}
			save()
			run('apply', '-auto-approve', '-input=false')
			run('plan', '-input=false', '-detailed-exitcode')
			del config['resource']['googleworkspace_org_unit']['temporary']
			save()
			run('apply', '-auto-approve', '-input=false')
			assert 'id:created' not in remote and any(row[0] == 'DELETE' for row in writes)
			run('plan', '-input=false', '-detailed-exitcode')
			mode['denied'] = True
			failure = run('plan', '-input=false', expected=1)
			assert secret not in failure.stderr and secret not in failure.stdout
			mode['denied'] = False
			env['GOOGLE_OAUTH_ACCESS_TOKEN'] = 'expired'
			failure = run('plan', '-input=false', expected=1)
			assert secret not in failure.stderr
		print('OU native lifecycle passed: import/clean refresh, root read, move/clear, protected deletion, create/delete, denial and expiry.')


if __name__ == '__main__': main()
