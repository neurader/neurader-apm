# neurader_callback.py
#
# Ansible callback plugin — installed automatically by `neurader init`.
# Captures every playbook run and writes a structured JSON log to
# /var/log/neurader/<playbook>_<date>_<time>.json
#
# To customise the log format, edit this file directly at its installed path.
# Run `sudo neurader reset-callback` to restore the default at any time.

from __future__ import absolute_import, division, print_function
__metaclass__ = type

DOCUMENTATION = '''
    callback: neurader
    type: notification
    short_description: Neurader Ansible execution monitor
    description:
      - Writes a structured JSON log file after each playbook run.
      - Captures per-host status (success / failed / unreachable).
      - Captures ALL failed tasks per host with full details.
      - Failed tasks include: task name, module, msg, stdout, stderr, rc, exception.
      - Automatically pushes logs to Loki after each run (if configured).
    requirements:
      - neurader binary installed at /usr/bin/neurader
'''

CALLBACK_NEEDS_ENABLED = False

from ansible.plugins.callback import CallbackBase

import datetime
import json
import os
import re
import shutil
import subprocess


def _load_config():
    defaults = {
        'log_dir': '/var/log/neurader',
        'retention_days': 3,
    }
    try:
        with open('/etc/neurader/neurader.conf') as f:
            cfg = json.load(f)
        defaults.update(cfg)
    except Exception:
        pass
    return defaults


def _find_neurader_bin():
    candidates = [
        '/usr/bin/neurader',
        '/usr/local/bin/neurader',
    ]
    for path in candidates:
        if os.path.isfile(path):
            return path
    found = shutil.which('neurader')
    if found:
        return found
    return None


_CFG          = _load_config()
LOG_DIR       = _CFG.get('log_dir', '/var/log/neurader')
NEURADER_BIN  = _find_neurader_bin()
SAFE_FILENAME = re.compile(r'[^\w\-.]')


class CallbackModule(CallbackBase):
    CALLBACK_VERSION       = 2.0
    CALLBACK_TYPE          = 'notification'
    CALLBACK_NAME          = 'neurader'
    CALLBACK_NEEDS_ENABLED = False

    def __init__(self):
        super(CallbackModule, self).__init__()
        self._playbook_name = None
        self._start_time    = None
        # host → { status, failed_tasks: [] }
        self._host_results  = {}

    # ── Playbook lifecycle ────────────────────────────────────────────────

    def v2_playbook_on_start(self, playbook):
        self._playbook_name = os.path.basename(playbook._file_name)
        self._start_time    = datetime.datetime.utcnow().isoformat()
        self._host_results  = {}

    # ── Per-task outcomes ─────────────────────────────────────────────────

    def v2_runner_on_ok(self, result):
        host = result._host.get_name()
        if host not in self._host_results:
            self._host_results[host] = {
                'status':       'success',
                'failed_tasks': [],
            }

    def v2_runner_on_failed(self, result, ignore_errors=False):
        host = result._host.get_name()
        task_detail = self._extract_task_detail(result)

        if host not in self._host_results:
            self._host_results[host] = {
                'status':       'failed',
                'failed_tasks': [],
            }
        else:
            self._host_results[host]['status'] = 'failed'

        self._host_results[host]['failed_tasks'].append(task_detail)

    def v2_runner_on_unreachable(self, result):
        host = result._host.get_name()
        r    = result._result
        self._host_results[host] = {
            'status': 'unreachable',
            'failed_tasks': [{
                'task_name': result._task.get_name(),
                'task_path': self._task_path(result),
                'module':    'connection',
                'msg':       r.get('msg', 'Host unreachable'),
                'stdout':    '',
                'stderr':    '',
                'rc':        -1,
                'exception': '',
            }],
        }

    def v2_runner_on_skipped(self, result):
        host = result._host.get_name()
        if host not in self._host_results:
            self._host_results[host] = {
                'status':       'success',
                'failed_tasks': [],
            }

    # ── Final stats ───────────────────────────────────────────────────────

    def v2_playbook_on_stats(self, stats):
        end_time = datetime.datetime.utcnow().isoformat()
        hosts    = sorted(stats.processed.keys())

        final_hosts = {}
        for host in hosts:
            summary = stats.summarize(host)
            entry   = self._host_results.get(host, {
                'status':       'success',
                'failed_tasks': [],
            })
            final_hosts[host] = {
                'status':       entry['status'],
                'failed_tasks': entry.get('failed_tasks', []),
                'summary': {
                    'ok':          summary.get('ok', 0),
                    'failures':    summary.get('failures', 0),
                    'unreachable': summary.get('unreachable', 0),
                    'changed':     summary.get('changed', 0),
                    'skipped':     summary.get('skipped', 0),
                },
            }

        payload = {
            'playbook':    self._playbook_name,
            'start_time':  self._start_time,
            'end_time':    end_time,
            'total_hosts': len(hosts),
            'hosts':       final_hosts,
        }

        log_path = self._write_log(payload)
        if log_path:
            self._display.display(
                '[neurader] Run logged → {}'.format(log_path))

        self._trigger_post_run()

    # ── Helpers ───────────────────────────────────────────────────────────

    def _extract_task_detail(self, result):
        """Extract full task details from a failed result."""
        r = result._result
        return {
            'task_name': result._task.get_name(),
            'task_path': self._task_path(result),
            'module':    result._task.action,
            'msg':       r.get('msg', ''),
            'stdout':    r.get('stdout', '') or r.get('module_stdout', ''),
            'stderr':    r.get('stderr', '') or r.get('module_stderr', ''),
            'rc':        r.get('rc', -1),
            'exception': r.get('exception', ''),
            'task_args': self._extract_task_args(result),
        }

    def _task_path(self, result):
        """Get the file path and line number of the task."""
        try:
            path = result._task.get_path()
            return path if path else ''
        except Exception:
            return ''

    def _extract_task_args(self, result):
        """Extract resolved task arguments (variables already substituted by Ansible)."""
        try:
            args = result._task.args.copy()
            # Scrub sensitive keys
            for key in list(args.keys()):
                if any(s in key.lower() for s in ('password', 'secret', 'token', 'key', 'pass')):
                    args[key] = '***'
            return args
        except Exception:
            return {}

    def _write_log(self, payload):
        try:
            os.makedirs(LOG_DIR, exist_ok=True)
            safe      = SAFE_FILENAME.sub('_', self._playbook_name or 'unknown')
            timestamp = datetime.datetime.utcnow().strftime('%Y-%m-%d_%H-%M-%S')
            filename  = '{}_{}.json'.format(safe, timestamp)
            path      = os.path.join(LOG_DIR, filename)
            with open(path, 'w') as f:
                json.dump(payload, f, indent=2)
            return path
        except Exception as exc:
            self._display.warning(
                '[neurader] Failed to write log: {}'.format(exc))
            return None

    def _trigger_post_run(self):
        """Invoke `neurader post-run` in the background (non-blocking)."""
        if not NEURADER_BIN:
            self._display.warning(
                '[neurader] binary not found — skipping post-run (Loki push)')
            return
        try:
            subprocess.Popen(
                [NEURADER_BIN, 'post-run'],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
        except Exception as exc:
            self._display.warning(
                '[neurader] post-run trigger failed: {}'.format(exc))
