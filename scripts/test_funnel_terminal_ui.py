import io
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'.hypershell/execute-sdlc'))
import terminal_ui
import funnel_dispatch as funnel


class Terminal(io.StringIO):
    def isatty(self):
        return True


class TerminalTests(unittest.TestCase):
    def test_plain_output_has_no_cursor_controls_or_color(self):
        stream=io.StringIO()
        ui=terminal_ui.TerminalUI(stream)
        ui.event('failure details')
        ui.draw(['dashboard'])
        self.assertEqual(stream.getvalue(),'failure details\n')

    @patch.dict(os.environ,{'TERM':'xterm-256color','NO_COLOR':'1'})
    @patch.object(terminal_ui.shutil,'get_terminal_size',return_value=os.terminal_size((35,12)))
    def test_panel_fits_terminal_and_clears_before_messages(self, size):
        stream=Terminal()
        ui=terminal_ui.TerminalUI(stream)
        ui.draw(['HYPERSHELL', 'x'*100])
        self.assertLessEqual(max(map(len,stream.getvalue().splitlines())),34)
        ui.event('Worker finished')
        self.assertIn('\033[2A\r\033[J',stream.getvalue())
        self.assertEqual(ui.lines,0)
        self.assertNotIn('\033[1;',stream.getvalue())

    @patch.dict(os.environ,{'TERM':'xterm-256color','NO_COLOR':'1'})
    @patch.object(terminal_ui.shutil,'get_terminal_size',return_value=os.terminal_size((35,12)))
    def test_ctrl_c_prints_complete_failed_attempt_paths_even_after_successful_retry(self, size):
        with tempfile.TemporaryDirectory() as tmp:
            store=funnel.Store(Path(tmp)/'state.db')
            self.addCleanup(store.db.close)
            job=dict(key='1:code-implementation',scope='agent/scope/test',agent='code-implementation',
                     item='1',fingerprint='input',url='https://example.test/1',pr_number='',pr_url='')
            long_path='/tmp/'+('long-folder-name/'*10)+'failed-attempt.log'
            run=store.start(job,long_path,100)
            store.finish(job['key'],run,7,60,3600,now=101)
            run=store.start(job,'/tmp/successful-retry.log',200)
            store.finish(job['key'],run,0,60,3600,now=201)
            args=funnel.parse_args(['agent/scope/test','--dry-run','--target-repo',tmp])
            coordinator=funnel.Coordinator(args,store,-1)
            stream=Terminal()
            with patch.object(sys,'stderr',stream), patch.object(coordinator,'discover',side_effect=KeyboardInterrupt):
                funnel.ui.draw(['HYPERSHELL','Logs: '+long_path])
                with self.assertRaises(KeyboardInterrupt):
                    coordinator.run()
            self.assertIn('Failed run logs:',stream.getvalue())
            self.assertIn(long_path,stream.getvalue().splitlines())
            self.assertNotIn('/tmp/successful-retry.log',stream.getvalue())
            self.assertEqual(funnel.ui.lines,0)

    def test_exit_includes_running_log_paths(self):
        with tempfile.TemporaryDirectory() as tmp:
            store=funnel.Store(Path(tmp)/'state.db')
            self.addCleanup(store.db.close)
            job=dict(key='2:code-review',scope='agent/scope/test',agent='code-review',item='2',fingerprint='input')
            store.start(job,'/tmp/still-running.log',100)
            args=funnel.parse_args(['agent/scope/test','--dry-run','--target-repo',tmp])
            coordinator=funnel.Coordinator(args,store,-1)
            stream=io.StringIO()
            with patch.object(sys,'stderr',stream):
                coordinator.show_exit_logs()
            self.assertIn('/tmp/still-running.log',stream.getvalue().splitlines())

    @patch.dict(os.environ, {'TERM':'xterm-256color','NO_COLOR':'1'})
    @patch.object(terminal_ui.shutil, 'get_terminal_size', return_value=os.terminal_size((80,24)))
    def test_dashboard_keeps_next_action_retry_and_exit_hint_visible(self, size):
        stream=Terminal()
        ui=terminal_ui.TerminalUI(stream)
        with patch.object(terminal_ui.time, 'time', return_value=200):
            ui.dashboard(scope='agent/scope/test', mode='MONITORING',
                         message='Scheduling resumes automatically after the existing owner exits.',
                         rows=[dict(item='1',agent='code-review',status='running',started=100,next_at=0),
                               dict(item='2',agent='triage',status='failed',started=100,next_at=290)],
                         capacity=2, gateway='test', footer='Ctrl-C detaches and prints logs.')
        rendered=stream.getvalue()
        self.assertIn('MONITORING',rendered)
        self.assertIn('Workers [#.] 1/2',rendered)
        self.assertIn('1m 40s elapsed',rendered)
        self.assertIn('Retry in 1m 30s',rendered)
        self.assertIn('Ctrl-C detaches and prints logs.',rendered)
        self.assertLessEqual(max(map(len,rendered.splitlines())),79)

    def test_plain_flag_disables_live_output_even_in_terminal(self):
        stream=Terminal()
        ui=terminal_ui.TerminalUI(stream)
        ui.plain=True
        ui.event('Monitoring work')
        ui.draw(['hidden'])
        self.assertEqual(stream.getvalue(),'Monitoring work\n')

    def test_title_truncation_preserves_both_ends(self):
        self.assertEqual(terminal_ui.middle_ellipsis('This is the title of the issue', 25),
                         'This is the...f the issue')
        self.assertEqual(terminal_ui.middle_ellipsis('Short', 20), 'Short')
        self.assertEqual(len(terminal_ui.middle_ellipsis('long title', 2)), 2)

    def test_activity_uses_latest_known_phase_without_exposing_log_contents(self):
        with tempfile.TemporaryDirectory() as tmp:
            log=Path(tmp)/'worker.log'
            log.write_text('Creating sandbox: example\nBootstrapping sandbox\n')
            self.assertEqual(terminal_ui.run_activity(log), 'Bootstrapping sandbox')
            log.write_text('Running agent\n  ⚙ Bash: SECRET=do-not-display\n')
            self.assertEqual(terminal_ui.run_activity(log), 'Agent working')
            log.write_text('Running agent\nExtracting output files\n')
            self.assertEqual(terminal_ui.run_activity(log), 'Extracting results')

    def test_remote_escape_sequences_are_removed_from_messages(self):
        self.assertEqual(terminal_ui.clean('hello\033[2J\rworld'),'helloworld')
