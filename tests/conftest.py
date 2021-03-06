from tests.kubernetes.minikube import *
from tests.kubernetes.utils import *
import docker
import itertools
import json
import os
import pytest
import random
import re
import semver
import string
import subprocess
import sys
import time
import urllib.request

SCRIPTS_DIR = os.path.join(os.path.dirname(os.path.realpath(__file__)), "..", "scripts")
K8S_MIN_VERSION = '1.7.0'
K8S_MAX_VERSION = '1.10.0'
K8S_DEFAULT_VERSION = '1.10.0'
K8S_DEFAULT_TIMEOUT = 300
K8S_DEFAULT_TEST_TIMEOUT = 120
K8S_DEFAULT_AGENT_IMAGE_NAME = "quay.io/signalfx/signalfx-agent-dev"
try:
    proc = subprocess.run(os.path.join(SCRIPTS_DIR, "current-version"), shell=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    K8S_DEFAULT_AGENT_IMAGE_TAG = proc.stdout.decode('utf-8').strip()
except:
    K8S_DEFAULT_AGENT_IMAGE_TAG = "latest"


def get_k8s_supported_versions():
    k8s_releases_json = None
    attempt = 0
    while attempt < 3:
        try:
            with urllib.request.urlopen('https://storage.googleapis.com/minikube/k8s_releases.json') as f:
                k8s_releases_json = json.loads(f.read().decode('utf-8'))
            break
        except:
            time.sleep(5)
            k8s_releases_json = None
            attempt += 1
    if not k8s_releases_json:
        print("Failed to get K8S releases from https://storage.googleapis.com/minikube/k8s_releases.json !")
        sys.exit(1)
    versions = []
    for r in k8s_releases_json:
        version = r['version'].strip().strip('v')
        if semver.match(version, '>=' + K8S_MIN_VERSION) and semver.match(version, '<=' + K8S_MAX_VERSION):
            versions.append(version)
    return sorted(versions, key=lambda v: list(map(int, v.split('.'))), reverse=True)

K8S_SUPPORTED_VERSIONS = get_k8s_supported_versions()
K8S_MAJOR_MINOR_VERSIONS = [v for v in K8S_SUPPORTED_VERSIONS if semver.parse_version_info(v).patch == 0]

K8S_SUPPORTED_OBSERVERS = ["k8s-api", "k8s-kubelet"]
K8S_DEFAULT_OBSERVERS = ["k8s-api", "k8s-kubelet"]


def pytest_addoption(parser):
    parser.addoption(
        "--k8s-version",
        action="store",
        default=K8S_DEFAULT_VERSION,
        help="K8S cluster version for minikube to deploy (default=%s). This option is ignored if the --k8s-container option also is specified." % K8S_DEFAULT_VERSION
    )
    parser.addoption(
        "--k8s-observers",
        action="store",
        default=",".join(K8S_DEFAULT_OBSERVERS),
        help="Comma-separated string of observers to test monitors with endpoints for the SignalFx agent (default=%s). Use '--k8s-observers=all' to test all supported observers." % ",".join(K8S_DEFAULT_OBSERVERS)
    )
    parser.addoption(
        "--k8s-timeout",
        action="store",
        default=K8S_DEFAULT_TIMEOUT,
        help="Timeout (in seconds) to wait for the minikube cluster to be ready (default=%d)." % K8S_DEFAULT_TIMEOUT
    )
    parser.addoption(
        "--k8s-agent-name",
        action="store",
        default=K8S_DEFAULT_AGENT_IMAGE_NAME,
        help="SignalFx agent image name to use for K8S tests (default=%s). The image must exist either locally or remotely." % K8S_DEFAULT_AGENT_IMAGE_NAME
    )
    parser.addoption(
        "--k8s-agent-tag",
        action="store",
        default=K8S_DEFAULT_AGENT_IMAGE_TAG,
        help="SignalFx agent image tag to use for K8S tests (default=%s). The image must exist either locally or remotely." % K8S_DEFAULT_AGENT_IMAGE_TAG
    )
    parser.addoption(
        "--k8s-test-timeout",
        action="store",
        default=K8S_DEFAULT_TEST_TIMEOUT,
        help="Timeout (in seconds) for each K8S test (default=%d)." % K8S_DEFAULT_TEST_TIMEOUT
    )
    parser.addoption(
        "--k8s-container",
        action="store",
        default=None,
        help="Name of a running minikube container to use for the tests (the container should not have the agent or any services already running). If not specified, a new minikube container will automatically be deployed."
    )
    parser.addoption(
        "--k8s-skip-teardown",
        action="store_true",
        help="If specified, the minikube container will not be stopped/removed when the tests complete."
    )


def pytest_generate_tests(metafunc):
    if 'minikube' in metafunc.fixturenames:
        k8s_version = metafunc.config.getoption("--k8s-version")
        if not k8s_version:
            version_to_test = K8S_DEFAULT_VERSION
        elif k8s_version.lower() == "latest":
            version_to_test = [K8S_SUPPORTED_VERSIONS[0]]
        else:
            assert k8s_version.strip('v') in K8S_SUPPORTED_VERSIONS, "K8S version \"%s\" not supported!" % k8s_version
            version_to_test = k8s_version
        metafunc.parametrize("minikube", [version_to_test], ids=["v%s" % v.strip('v') for v in [version_to_test]], scope="session", indirect=True)
    if 'k8s_observer' in metafunc.fixturenames:
        k8s_observers = metafunc.config.getoption("--k8s-observers")
        if not k8s_observers:
            observers_to_test = K8S_DEFAULT_OBSERVERS
        elif k8s_observers.lower() == 'all':
            observers_to_test = K8S_SUPPORTED_OBSERVERS
        else:
            for o in k8s_observers.split(','):
                assert o in K8S_SUPPORTED_OBSERVERS, "observer \"%s\" not supported!" % o
            observers_to_test = k8s_observers.split(',')
        metafunc.parametrize("k8s_observer", observers_to_test, ids=[o for o in observers_to_test], indirect=True)


@pytest.fixture(scope="session")
def minikube(request, worker_id):
    def teardown():
        if not k8s_skip_teardown:
            try:
                print("Removing minikube container ...")
                mk.container.remove(force=True, v=True)
            except:
                pass

    request.addfinalizer(teardown)
    k8s_version = "v" + request.param.lstrip("v")
    k8s_timeout = int(request.config.getoption("--k8s-timeout"))
    k8s_container = request.config.getoption("--k8s-container")
    k8s_skip_teardown = request.config.getoption("--k8s-skip-teardown")
    mk = Minikube()
    mk.worker_id = worker_id
    if k8s_container:
        mk.connect(k8s_container, k8s_timeout)
        k8s_skip_teardown = True
    elif worker_id == "master" or worker_id == "gw0":
        mk.deploy(k8s_version, k8s_timeout)
        if worker_id == "gw0":
            k8s_skip_teardown = True
    else:
        mk.connect("minikube", k8s_timeout, version=k8s_version)
        k8s_skip_teardown = True
    return mk


@pytest.fixture(scope="session")
def registry(minikube, worker_id):
    def get_registry_logs():
        cont.reload()
        return cont.logs().decode('utf-8')

    cont = None
    port = None
    if worker_id == "master" or worker_id == "gw0":
        minikube.start_registry()
        port = minikube.registry_port
    print("\nWaiting for registry to be ready ...")
    assert wait_for(p(container_is_running, minikube.client, "registry"), timeout_seconds=60), \
        "timed out waiting for registry container to start!"
    cont = minikube.client.containers.get("registry")
    assert wait_for(lambda: has_log_message(get_registry_logs(), message="listening on [::]:"), timeout_seconds=30), \
        "timed out waiting for registry to be ready!"
    if not port:
        match = re.search('listening on \[::\]:(\d+)', get_registry_logs())
        assert match, "failed to determine registry port!"
        port = int(match.group(1))
    return {"container": cont, "port": port}


@pytest.fixture(scope="session")
def agent_image(minikube, registry, request, worker_id):
    def teardown():
        try:
            client.images.remove("%s:%s" % (agent_image_name, agent_image_tag))
        except:
            pass

    request.addfinalizer(teardown)
    client = get_docker_client()
    final_agent_image_name = request.config.getoption("--k8s-agent-name")
    final_agent_image_tag = request.config.getoption("--k8s-agent-tag")
    agent_image_name = "localhost:%d/%s" % (registry['port'], final_agent_image_name.split("/")[-1])
    agent_image_tag = final_agent_image_tag
    if worker_id == "master" or worker_id == "gw0":
        if not has_docker_image(client, final_agent_image_name, final_agent_image_tag):
            print("\nAgent image '%s:%s' not found in local registry." % (final_agent_image_name, final_agent_image_tag))
            print("\nAttempting to pull from remote registry ...")
            final_agent_image = client.images.pull(final_agent_image_name, tag=final_agent_image_tag)
        else:
            final_agent_image = client.images.get(final_agent_image_name + ":" + final_agent_image_tag)
        print("\nTagging %s:%s as %s:%s ..." % (final_agent_image_name, final_agent_image_tag, agent_image_name, agent_image_tag))
        final_agent_image.tag(agent_image_name, tag=agent_image_tag)
        print("\nPushing %s:%s ..." % (agent_image_name, agent_image_tag))
        client.images.push(agent_image_name, tag=agent_image_tag)
        print("\nPulling %s:%s ..." % (agent_image_name, agent_image_tag))
        minikube.pull_agent_image(agent_image_name, agent_image_tag, final_agent_image.id)
    else:
        print("\nWaiting for agent image \"%s:%s\" to be pulled to minikube ..." % (agent_image_name, agent_image_tag))
        assert wait_for(p(has_docker_image, minikube.client, agent_image_name, agent_image_tag), timeout_seconds=60), \
            "timed out waiting for agent image \"%s:%s\"!" % (agent_image_name, agent_image_tag)
        final_agent_image = client.images.get(final_agent_image_name + ":" + final_agent_image_tag)
    return {"name": agent_image_name, "tag": agent_image_tag, "id": final_agent_image.id}


@pytest.fixture
def k8s_observer(request):
    return request.param


@pytest.fixture
def k8s_test_timeout(request):
    return int(request.config.getoption("--k8s-test-timeout"))


@pytest.fixture
def k8s_namespace(worker_id):
    chars = string.ascii_lowercase + string.digits
    namespace = worker_id + '-' + ''.join((random.choice(chars)) for x in range(8))
    return namespace
