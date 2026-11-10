#!/usr/bin/env bash
# Exercise admission rendering with gateway configuration, without a cluster or OCE checkout.
set -euo pipefail
python3 - "$(dirname -- "${BASH_SOURCE[0]}")/render-oce.py" <<'PY'
import json
import os
from pathlib import Path
import runpy
import sys
import tempfile
import unittest
from unittest.mock import patch

import yaml

renderer = Path(sys.argv.pop()).resolve()
gateway = 'openshell-0123456789abcdef'


class AdmissionImagesTest(unittest.TestCase):
    def render(self, state, driver, selected_namespace=gateway, discovered=(gateway,)):
        (state / 'images.json').write_text(json.dumps({'runtime': 'example.com/oce@sha256:' + 'a' * 64,
                                                     'controller': 'example.com/controller:test'}))
        config = '[openshell.drivers.kubernetes]\n' + '\n'.join(
            f'{key} = {json.dumps(value)}' for key, value in driver.items())
        resources = {
            ('get', 'deployments', '-A', '--field-selector', 'metadata.name=openshell-gateway'):
                {'items': [{'metadata': {'namespace': namespace}} for namespace in discovered]},
            ('-n', gateway, 'get', 'service', 'openshell-gateway'):
                {'spec': {'clusterIP': '10.96.0.10', 'selector': {'app': 'gateway'}}},
            ('-n', gateway, 'get', 'configmap', 'openshell-gateway-config'):
                {'data': {'gateway.toml': config}},
            ('-n', 'external-cloud-db', 'get', 'service', 'postgres'):
                {'spec': {'clusterIP': '10.96.0.20'}},
            ('-n', 'external-cloud-db', 'get', 'pods', '-l', 'app=postgres'):
                {'items': [{'status': {'podIP': '10.244.0.20'}}]},
            ('-n', 'default', 'get', 'service', 'kubernetes'):
                {'spec': {'clusterIP': '10.96.0.1'}},
            ('-n', 'default', 'get', 'endpointslices', '-l', 'kubernetes.io/service-name=kubernetes'):
                {'items': [{'endpoints': [{'addresses': ['172.18.0.2']}], 'ports': [{'port': 6443}]}]},
        }

        def output(command):
            if command[0] == 'kubectl':
                self.assertEqual(command[1:5], ['--kubeconfig', str(state / 'kubeconfig'),
                                                '--context', 'kind-hypershell-oce'])
                return json.dumps(resources[tuple(command[5:-2])]).encode()
            self.assertEqual(command[0], 'jq')
            if Path(command[-1]).name == 'installation.jq':
                return json.dumps({'backend': [{'configuration': {}}], 'drivers': {
                    'compute': {'configuration': {'network': {'providerHarness': {}}, 'runtime': {}}},
                    'sandbox': {'configuration': {'gateway': {'operatorWorkspaceResources': []}}}}}).encode()
            self.assertEqual(Path(command[-1]).name, 'values.jq')
            return b'{"cluster": {}}'

        with patch.dict(os.environ, {'OCE_KIND_STATE': str(state), 'OCE_REPO': str(state),
                                     'GATEWAY_NAMESPACE': selected_namespace}), \
                patch('subprocess.check_output', side_effect=output):
            runpy.run_path(str(renderer), run_name='__main__')

    def test_rediscovers_live_gateway_instead_of_using_stale_saved_namespace(self):
        with tempfile.TemporaryDirectory() as directory:
            state = Path(directory)
            (state / 'gateway-namespace').write_text('openshell-ffffffffffffffff\n')
            self.render(state, {'service_account_name': 'sandbox',
                                'supervisor_image': 'example.com/supervisor:test',
                                'sandbox_runtime_image': 'example.com/bootstrap:test'}, selected_namespace='')
            self.assertEqual((state / 'gateway-namespace').read_text(), gateway + '\n')
            installation = json.loads((state / 'installation.json').read_text())
            self.assertEqual(installation['backend'][0]['configuration']['endpoint'],
                             f'https://openshell-gateway.{gateway}.svc:8080')

    def test_discovery_rejects_missing_or_ambiguous_gateways(self):
        for discovered in ((), (gateway, 'openshell-ffffffffffffffff')):
            with self.subTest(discovered=discovered), tempfile.TemporaryDirectory() as directory:
                state = Path(directory)
                with self.assertRaisesRegex(SystemExit, 'Expected exactly one OpenShell gateway'):
                    self.render(state, {}, selected_namespace='', discovered=discovered)
                self.assertFalse((state / 'runtime-admission.yaml').exists())
                self.assertFalse((state / 'gateway-namespace').exists())

    def test_explicit_namespace_is_validated_before_rendering(self):
        with tempfile.TemporaryDirectory() as directory:
            state = Path(directory)
            with self.assertRaisesRegex(SystemExit, 'HyperShell-created'):
                self.render(state, {}, selected_namespace="invalid'namespace")
            self.assertFalse((state / 'runtime-admission.yaml').exists())

    def test_uses_configured_tags_and_digests(self):
        for suffix in (':release-a', ':release-b', '@sha256:' + 'b' * 64):
            with self.subTest(suffix=suffix), tempfile.TemporaryDirectory() as directory:
                state = Path(directory)
                driver = {'service_account_name': 'sandbox',
                          'supervisor_image': 'example.com/supervisor' + suffix,
                          'sandbox_runtime_image': 'example.com/bootstrap' + suffix}
                self.render(state, driver)
                policy, binding = yaml.safe_load_all((state / 'runtime-admission.yaml').read_text())
                self.assertEqual(policy['spec']['failurePolicy'], 'Fail')
                self.assertEqual(binding['spec']['validationActions'], ['Deny'])
                expression = policy['spec']['validations'][3]['expression']
                self.assertIn("'example.com/oce@sha256:" + 'a' * 64 + "'", expression)
                self.assertIn(repr(driver['supervisor_image']), expression)
                self.assertIn("c.name == 'openshell-sandbox-bootstrap' && c.image == "
                              + repr(driver['sandbox_runtime_image']), expression)
                self.assertNotIn('__', (state / 'runtime-admission.yaml').read_text())

    def test_missing_or_unsafe_configured_image_fails_closed(self):
        for field in ('supervisor_image', 'sandbox_runtime_image'):
            for value in (None, '', "example.com/image' || true || '"):
                with self.subTest(field=field, value=value), tempfile.TemporaryDirectory() as directory:
                    state = Path(directory)
                    driver = {'service_account_name': 'sandbox',
                              'supervisor_image': 'example.com/supervisor:test',
                              'sandbox_runtime_image': 'example.com/bootstrap:test'}
                    if value is None:
                        del driver[field]
                    else:
                        driver[field] = value
                    with self.assertRaises((KeyError, ValueError)):
                        self.render(state, driver)
                    self.assertFalse((state / 'runtime-admission.yaml').exists())


unittest.main()
PY
