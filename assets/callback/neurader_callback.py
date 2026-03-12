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

        # If ignore_errors: true is set on the task, the playbook continues
        # but the failure is still real — record it with a note so engineers
        # can see what was silently swallowed.
        if ignore_errors:
            task_detail['task_name'] = '[ignored] ' + task_detail['task_name']

        if host not in self._host_results:
            self._host_results[host] = {
                'status':       'failed',
                'failed_tasks': [],
            }
        else:
            self._host_results[host]['status'] = 'failed'

        self._host_results[host]['failed_tasks'].append(task_detail)

    def v2_runner_item_on_failed(self, result):
        """Capture individual loop item failures.

        When a task loops over a list (with_items / loop), Ansible fires this
        callback for each item that fails rather than v2_runner_on_failed.
        We record each failed item as a separate failed_task entry so that
        '- name: install packages\n  loop: [nginx, redis, doesnotexist]'
        captures exactly which item failed, not just the task name.
        """
        host = result._host.get_name()

        r    = result._result
        item = r.get('item', '')
        # item may be a dict (e.g. loop over dicts) — stringify for display
        if isinstance(item, dict):
            item_label = ', '.join('{}={}'.format(k, v) for k, v in item.items())
        else:
            item_label = str(item) if item != '' else ''

        task_name = result._task.get_name()
        if item_label:
            task_name = '{} [item: {}]'.format(task_name, item_label)

        detail = {
            'task_name': task_name,
            'task_path': self._task_path(result),
            'module':    result._task.action,
            'msg':       r.get('msg', ''),
            'stdout':    r.get('stdout', '') or r.get('module_stdout', ''),
            'stderr':    r.get('stderr', '') or r.get('module_stderr', ''),
            'rc':        r.get('rc', -1),
            'exception': r.get('exception', ''),
            'task_args': self._extract_task_args(result),
            'loop_item': item_label,
        }

        if host not in self._host_results:
            self._host_results[host] = {'status': 'failed', 'failed_tasks': []}
        else:
            self._host_results[host]['status'] = 'failed'

        self._host_results[host]['failed_tasks'].append(detail)

    def v2_runner_on_async_failed(self, result):
        """Capture async task failures (tasks run with async: N).

        Ansible does not call v2_runner_on_failed for async tasks — it fires
        this callback instead when the async job result is polled and found
        to have failed.
        """
        host = result._host.get_name()
        task_detail = self._extract_task_detail(result)
        task_detail['task_name'] = '[async] ' + task_detail['task_name']

        if host not in self._host_results:
            self._host_results[host] = {'status': 'failed', 'failed_tasks': []}
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
        """Extract full task details from a failed result.

        Handles module-specific error formats:
        - yum/dnf:  per-package errors in r['failures'] list
        - apt:      errors in r['msg'] / r['stderr']
        - loop:     failed items extracted from r['results'] list
        - command/shell: stdout + stderr + rc
        - general:  r['msg'], r['stderr'], r['exception']
        """
        r = result._result

        msg      = r.get('msg', '')
        failures = r.get('failures', [])
        results  = r.get('results', [])

        details = []

        # yum/dnf: explicit failures list
        for f in failures:
            if f and f not in details:
                details.append(str(f))

        # results list — used by yum/dnf for per-package output AND by loop
        # tasks when all items fail at once (non-item loop result).
        # We extract any entry that has failed=True or contains an error string.
        for item in results:
            if isinstance(item, dict):
                # loop item dict: {'failed': True, 'msg': '...', 'item': '...'}
                if item.get('failed') or item.get('rc', 0) != 0:
                    item_label = item.get('item', '')
                    if isinstance(item_label, dict):
                        item_label = str(item_label)
                    item_msg = item.get('msg', '') or item.get('stderr', '')
                    entry = '[item: {}] {}'.format(item_label, item_msg).strip()
                    if entry and entry not in details:
                        details.append(entry)
            elif isinstance(item, str):
                # plain string results (yum legacy format)
                if ('No package' in item or 'Error' in item) and item not in details:
                    details.append(item)

        # Append specific details to msg for full clarity
        if details:
            specific = '\n'.join(details)
            if specific not in msg:
                msg = (msg + '\n' + specific) if msg else specific

        return {
            'task_name': result._task.get_name(),
            'task_path': self._task_path(result),
            'module':    result._task.action,
            'msg':       msg,
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
