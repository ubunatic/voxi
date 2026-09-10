import { Extension, gettext as _ } from 'resource:///org/gnome/shell/extensions/extension.js';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import * as PanelMenu from 'resource:///org/gnome/shell/ui/panelMenu.js';
import * as PopupMenu from 'resource:///org/gnome/shell/ui/popupMenu.js';
import * as Slider from 'resource:///org/gnome/shell/ui/slider.js';
import GObject from 'gi://GObject';
import St from 'gi://St';
import Clutter from 'gi://Clutter';
import GLib from 'gi://GLib';
import Gio from 'gi://Gio';
import Meta from 'gi://Meta';

async function runCommand(programName, args, cancellable = null) {
    let program = GLib.find_program_in_path(programName);
    if (!program) {
        const home = GLib.get_home_dir();
        const candidates = [
            GLib.build_filenamev([home, '.local', 'bin', programName]),
            GLib.build_filenamev([home, 'go', 'bin', programName]),
            GLib.build_filenamev([home, 'projects', 'voxi', programName]),
            `/usr/local/bin/${programName}`,
            `/usr/bin/${programName}`,
        ];
        for (const cand of candidates) {
            if (GLib.file_test(cand, GLib.FileTest.IS_EXECUTABLE)) {
                program = cand;
                break;
            }
        }
        if (!program) {
            program = programName;
        }
    }

    try {
        const launcher = new Gio.SubprocessLauncher({
            flags: Gio.SubprocessFlags.STDOUT_PIPE | Gio.SubprocessFlags.STDERR_PIPE,
        });
        const proc = launcher.spawnv([program, ...args]);
        const [ok, stdoutStr, stderrStr] = await new Promise((resolve, reject) => {
            proc.communicate_utf8_async(null, cancellable, (obj, res) => {
                try {
                    const result = obj.communicate_utf8_finish(res);
                    resolve(result);
                } catch (e) {
                    reject(e);
                }
            });
        });
        const res = {
            success: proc.get_successful() && ok,
            exitCode: proc.get_exit_status(),
            stdout: stdoutStr ? stdoutStr.trim() : '',
            stderr: stderrStr ? stderrStr.trim() : '',
        };
        return res;
    } catch (err) {
        return {
            success: false,
            exitCode: -1,
            stdout: '',
            stderr: err.message || String(err),
        };
    }
}

async function runVoxi(args, cancellable = null) {
    return runCommand('voxi', args, cancellable);
}

export default class VoxiVoiceInputExtension extends Extension {
    enable() {
        this._capturedWindow = null;
        this._typeDelayMs = 0;
        this._currentMode = 'unknown';

        // 1. Create Top Bar Indicator Button
        this._indicator = new PanelMenu.Button(0.0, this.metadata.name, false);
        this._icon = new St.Icon({
            icon_name: 'audio-input-microphone-symbolic',
            style_class: 'system-status-icon',
        });
        this._indicator.add_child(this._icon);

        // Intercept menu.toggle: if recording, left-click triggers 1-click stop instead of opening menu
        const origToggle = this._indicator.menu.toggle.bind(this._indicator.menu);
        this._indicator.menu.toggle = () => {
            if (this._isRecording) {
                this._stopRecordingDirectly();
                return;
            }
            origToggle();
        };

        // 2. Build Menu Layout
        this._buildMenu();

        // 3. Connect Menu open-state for focus capture and data refresh
        this._openStateSignal = this._indicator.menu.connect('open-state-changed', (menu, isOpen) => {
            if (isOpen) {
                this._onMenuOpened();
            } else {
                this._onMenuClosed();
            }
        });

        // Add indicator to top bar
        Main.panel.addToStatusArea(this.uuid, this._indicator, 1, 'right');

        // 4. Start background state poller (polls every 500ms)
        this._pollTimerId = GLib.timeout_add(GLib.PRIORITY_DEFAULT, 500, () => {
            this._checkBackgroundState();
            return GLib.SOURCE_CONTINUE;
        });

        try {
            Main.notify('Voxi Voice Input', 'Voice Input companion extension activated');
        } catch (e) {}
    }

    disable() {
        if (this._pollTimerId) {
            GLib.source_remove(this._pollTimerId);
            this._pollTimerId = 0;
        }
        if (this._openStateSignal && this._indicator) {
            this._indicator.menu.disconnect(this._openStateSignal);
            this._openStateSignal = 0;
        }
        if (this._indicator) {
            this._indicator.destroy();
            this._indicator = null;
        }
        this._capturedWindow = null;
    }

    _checkBackgroundState() {
        const runtimeDir = GLib.get_user_runtime_dir();
        const stateFile = GLib.build_filenamev([runtimeDir, 'voxtype', 'state']);
        let isRecording = false;

        if (GLib.file_test(stateFile, GLib.FileTest.EXISTS)) {
            try {
                const [ok, contents] = GLib.file_get_contents(stateFile);
                if (ok && contents) {
                    const text = new TextDecoder().decode(contents).trim().toLowerCase();
                    isRecording = text.includes('recording') || text.includes('listening');
                }
            } catch (e) {}
        }

        if (this._isRecording !== isRecording) {
            this._isRecording = isRecording;
            this._updateRecordingUI(isRecording);
        }
    }

    _updateRecordingUI(isRecording) {
        if (isRecording) {
            if (this._recordToggleBtn) {
                this._recordToggleBtn.label = '⏹ Stop Recording';
                this._recordToggleBtn.add_style_class_name('recording');
            }
            if (this._icon) {
                this._icon.icon_name = 'media-record-symbolic';
                this._icon.add_style_class_name('voice-input-icon-recording');
            }
        } else {
            if (this._recordToggleBtn) {
                this._recordToggleBtn.label = '🎙 Start Recording';
                this._recordToggleBtn.remove_style_class_name('recording');
            }
            if (this._icon) {
                this._icon.icon_name = 'audio-input-microphone-symbolic';
                this._icon.remove_style_class_name('voice-input-icon-recording');
            }
        }
    }

    _onMenuOpened() {
        this._capturedWindow = global.display.get_focus_window();
        this._refreshAllState();
    }

    _onMenuClosed() {}

    _buildMenu() {
        const menu = this._indicator.menu;

        // ── Section 0: Live Recording Toggle ──
        const recordSection = new PopupMenu.PopupBaseMenuItem({
            reactive: false,
            can_focus: false,
            style_class: 'voice-input-section',
        });
        const recordLayout = new St.BoxLayout({ vertical: true, x_expand: true });
        
        this._recordToggleBtn = new St.Button({
            label: '🎙 Start Recording',
            style_class: 'voice-input-record-btn',
            x_expand: true,
            can_focus: true,
        });
        this._recordToggleBtn.connect('clicked', () => this._toggleRecording());
        recordLayout.add_child(this._recordToggleBtn);
        recordSection.add_child(recordLayout);
        menu.addMenuItem(recordSection);

        menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        // ── Section 1: Mode Selection ──
        const modeSection = new PopupMenu.PopupBaseMenuItem({
            reactive: false,
            can_focus: false,
            style_class: 'voice-input-section',
        });
        const modeLayout = new St.BoxLayout({ vertical: true, x_expand: true });
        
        const modeHeaderBox = new St.BoxLayout({ x_expand: true });
        this._modeTitleLabel = new St.Label({
            text: 'Voice Dictation Mode',
            style_class: 'voice-input-section-title',
            y_align: Clutter.ActorAlign.CENTER,
        });
        this._modeStatusLabel = new St.Label({
            text: '(checking...)',
            style_class: 'voice-input-note',
            x_align: Clutter.ActorAlign.END,
            x_expand: true,
            y_align: Clutter.ActorAlign.CENTER,
        });
        modeHeaderBox.add_child(this._modeTitleLabel);
        modeHeaderBox.add_child(this._modeStatusLabel);
        modeLayout.add_child(modeHeaderBox);

        const modeBtnBox = new St.BoxLayout({
            style_class: 'voice-input-mode-box',
            x_expand: true,
        });
        
        this._batchModeBtn = new St.Button({
            label: 'Batch (End)',
            style_class: 'voice-input-mode-button',
            x_expand: true,
            can_focus: true,
        });
        this._batchModeBtn.connect('clicked', () => this._setMode('batch'));

        this._streamingModeBtn = new St.Button({
            label: 'Streaming (Partial)',
            style_class: 'voice-input-mode-button',
            x_expand: true,
            can_focus: true,
        });
        this._streamingModeBtn.connect('clicked', () => this._setMode('streaming'));

        this._eagerModeBtn = new St.Button({
            label: 'Eager (Continuous)',
            style_class: 'voice-input-mode-button',
            x_expand: true,
            can_focus: true,
        });
        this._eagerModeBtn.connect('clicked', () => this._setMode('eager'));

        modeBtnBox.add_child(this._batchModeBtn);
        modeBtnBox.add_child(this._streamingModeBtn);
        modeBtnBox.add_child(this._eagerModeBtn);
        modeLayout.add_child(modeBtnBox);

        this._modeDescLabel = new St.Label({
            text: 'Batch: Whisper base.en | Streaming: Parakeet | Eager: Cohere Transcribe',
            style_class: 'voice-input-note',
        });
        modeLayout.add_child(this._modeDescLabel);

        modeSection.add_child(modeLayout);
        menu.addMenuItem(modeSection);

        menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        // ── Section 2: Typing Speed (type_delay_ms) Slider ──
        const delaySection = new PopupMenu.PopupBaseMenuItem({
            reactive: false,
            can_focus: false,
            style_class: 'voice-input-section',
        });
        const delayLayout = new St.BoxLayout({ vertical: true, x_expand: true });

        const delayHeader = new St.BoxLayout({ x_expand: true });
        const delayTitle = new St.Label({
            text: 'Keystroke Delay',
            style_class: 'voice-input-section-title',
            y_align: Clutter.ActorAlign.CENTER,
        });
        this._delayValueLabel = new St.Label({
            text: '0 ms (instant)',
            style_class: 'voice-input-note',
            x_align: Clutter.ActorAlign.END,
            x_expand: true,
            y_align: Clutter.ActorAlign.CENTER,
        });
        delayHeader.add_child(delayTitle);
        delayHeader.add_child(this._delayValueLabel);
        delayLayout.add_child(delayHeader);

        const sliderBox = new St.BoxLayout({
            style_class: 'voice-input-slider-box',
            x_expand: true,
        });
        this._slider = new Slider.Slider(0.0);
        this._slider.x_expand = true;
        this._slider.connect('notify::value', () => this._onSliderValueChanged());
        this._slider.connect('drag-end', () => this._onSliderDragEnd());
        sliderBox.add_child(this._slider);
        delayLayout.add_child(sliderBox);

        const delayNote = new St.Label({
            text: '0ms instant for terminals; 10-20ms if browser drops characters',
            style_class: 'voice-input-note',
        });
        delayLayout.add_child(delayNote);

        delaySection.add_child(delayLayout);
        menu.addMenuItem(delaySection);

        menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        // ── Section 3: Recent Dictation History ──
        const historySection = new PopupMenu.PopupBaseMenuItem({
            reactive: false,
            can_focus: false,
            style_class: 'voice-input-section',
        });
        const historyLayout = new St.BoxLayout({ vertical: true, x_expand: true });

        const historyHeader = new St.BoxLayout({ x_expand: true });
        const historyTitle = new St.Label({
            text: 'Recent Dictations',
            style_class: 'voice-input-section-title',
            y_align: Clutter.ActorAlign.CENTER,
        });
        this._clearHistoryBtn = new St.Button({
            label: 'Clear History',
            style_class: 'voice-input-clear-btn',
            x_align: Clutter.ActorAlign.END,
            x_expand: true,
            can_focus: true,
        });
        this._clearHistoryBtn.connect('clicked', () => this._clearHistory());
        historyHeader.add_child(historyTitle);
        historyHeader.add_child(this._clearHistoryBtn);
        historyLayout.add_child(historyHeader);

        this._historyScroll = new St.ScrollView({
            style_class: 'voice-input-history-scroll',
            x_expand: true,
            vscrollbar_policy: St.PolicyType.AUTOMATIC,
            hscrollbar_policy: St.PolicyType.NEVER,
        });
        this._historyListBox = new St.BoxLayout({
            vertical: true,
            style_class: 'voice-input-history-box',
            x_expand: true,
        });
        this._historyScroll.set_child(this._historyListBox);
        historyLayout.add_child(this._historyScroll);

        historySection.add_child(historyLayout);
        menu.addMenuItem(historySection);
    }

    async _refreshAllState() {
        await Promise.all([
            this._refreshRecordingStatus(),
            this._refreshModeStatus(),
            this._refreshTypeDelay(),
            this._refreshHistory(),
        ]);
    }

    async _refreshRecordingStatus() {
        const res = await runVoxi(['record', 'status']);
        const isRec = res.success && res.stdout.toLowerCase().includes('recording');
        this._isRecording = isRec;
        this._updateRecordingUI(isRec);
    }

    async _stopRecordingDirectly() {
        const res = await runVoxi(['record', 'stop']);
        if (res.success) {
            this._isRecording = false;
            this._updateRecordingUI(false);
            try {
                Main.notify('Voxi Voice Input', 'Recording stopped');
            } catch (e) {}
        }
    }

    async _toggleRecording() {
        const action = this._isRecording ? 'stop' : 'toggle';
        if (this._recordToggleBtn) {
            this._recordToggleBtn.reactive = false;
            this._recordToggleBtn.label = '⏳ Switching...';
        }
        const res = await runVoxi(['record', action]);
        if (this._recordToggleBtn) {
            this._recordToggleBtn.reactive = true;
        }
        await this._refreshRecordingStatus();
    }

    async _refreshModeStatus() {
        const res = await runVoxi(['mode']);
        if (!res.success) {
            if (this._modeStatusLabel) {
                this._modeStatusLabel.text = '(service error)';
            }
            return;
        }
        const out = res.stdout.toLowerCase();
        let mode = 'neither';
        if (out.includes('batch')) {
            mode = 'batch';
        } else if (out.includes('streaming')) {
            mode = 'streaming';
        } else if (out.includes('eager')) {
            mode = 'eager';
        }
        this._currentMode = mode;
        this._updateModeUI(mode);
    }

    _updateModeUI(mode) {
        if (this._batchModeBtn) {
            if (mode === 'batch') {
                this._batchModeBtn.add_style_class_name('active');
            } else {
                this._batchModeBtn.remove_style_class_name('active');
            }
        }
        if (this._streamingModeBtn) {
            if (mode === 'streaming') {
                this._streamingModeBtn.add_style_class_name('active');
            } else {
                this._streamingModeBtn.remove_style_class_name('active');
            }
        }
        if (this._eagerModeBtn) {
            if (mode === 'eager') {
                this._eagerModeBtn.add_style_class_name('active');
            } else {
                this._eagerModeBtn.remove_style_class_name('active');
            }
        }
        if (this._modeStatusLabel) {
            this._modeStatusLabel.text = `(${mode})`;
        }
    }

    async _setMode(targetMode) {
        if (this._modeStatusLabel) {
            this._modeStatusLabel.text = `(switching to ${targetMode}...)`;
        }
        const res = await runVoxi(['mode', targetMode]);
        await this._refreshModeStatus();
    }

    async _refreshTypeDelay() {
        const res = await runVoxi(['config', 'get', 'type-delay-ms']);
        if (res.success) {
            const val = parseInt(res.stdout, 10);
            if (!isNaN(val)) {
                this._typeDelayMs = val;
                const frac = Math.min(Math.max(val / 50.0, 0.0), 1.0);
                if (this._slider) {
                    this._slider.value = frac;
                }
                this._updateDelayLabel(val);
            }
        }
    }

    _updateDelayLabel(ms) {
        if (this._delayValueLabel) {
            if (ms === 0) {
                this._delayValueLabel.text = '0 ms (instant)';
            } else {
                this._delayValueLabel.text = `${ms} ms`;
            }
        }
    }

    _onSliderValueChanged() {
        if (!this._slider) return;
        const ms = Math.round(this._slider.value * 50);
        this._updateDelayLabel(ms);
    }

    async _onSliderDragEnd() {
        if (!this._slider) return;
        const ms = Math.round(this._slider.value * 50);
        this._typeDelayMs = ms;
        await runVoxi(['config', 'set', 'type-delay-ms', String(ms)]);
    }

    async _refreshHistory() {
        if (!this._historyListBox) return;
        this._historyListBox.destroy_all_children();

        const res = await runVoxi(['history', 'list', '--format', 'json']);
        if (!res.success || !res.stdout) {
            this._historyListBox.add_child(new St.Label({
                text: 'No dictation history found.',
                style_class: 'voice-input-note',
            }));
            return;
        }

        try {
            const items = JSON.parse(res.stdout);
            if (!items || items.length === 0) {
                this._historyListBox.add_child(new St.Label({
                    text: 'No dictation history recorded yet.',
                    style_class: 'voice-input-note',
                }));
                return;
            }

            for (const item of items) {
                const itemBox = new St.BoxLayout({
                    vertical: true,
                    style_class: 'voice-input-history-item',
                    x_expand: true,
                });

                const topRow = new St.BoxLayout({ x_expand: true });
                const timeLabel = new St.Label({
                    text: this._formatTime(item.time),
                    style_class: 'voice-input-history-time',
                    y_align: Clutter.ActorAlign.CENTER,
                });
                topRow.add_child(timeLabel);

                const btnBox = new St.BoxLayout({
                    x_align: Clutter.ActorAlign.END,
                    x_expand: true,
                });

                const copyBtn = new St.Button({
                    label: '📋 Copy',
                    style_class: 'voice-input-action-btn',
                    can_focus: true,
                });
                copyBtn.connect('clicked', () => this._copyHistoryItem(item.id));
                btnBox.add_child(copyBtn);

                const retypeBtn = new St.Button({
                    label: '⌨ Retype',
                    style_class: 'voice-input-action-btn retype-btn',
                    can_focus: true,
                });
                retypeBtn.connect('clicked', () => this._retypeHistoryItem(item.id));
                btnBox.add_child(retypeBtn);

                topRow.add_child(btnBox);
                itemBox.add_child(topRow);

                const textLabel = new St.Label({
                    text: item.text,
                    style_class: 'voice-input-history-text',
                });
                textLabel.clutter_text.set_line_wrap(true);
                textLabel.clutter_text.set_line_wrap_mode(Clutter.WrapMode.WORD_CHAR);
                itemBox.add_child(textLabel);

                this._historyListBox.add_child(itemBox);
            }
        } catch (err) {
            this._historyListBox.add_child(new St.Label({
                text: 'Error parsing history entries.',
                style_class: 'voice-input-note',
            }));
        }
    }

    _formatTime(isoString) {
        try {
            const d = new Date(isoString);
            return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
        } catch (e) {
            return isoString;
        }
    }

    async _copyHistoryItem(id) {
        const res = await runVoxi(['history', 'copy', id]);
        if (res.success) {
            try {
                Main.notify('Voxi Voice Input', 'Transcript copied to clipboard');
            } catch (e) {}
        }
    }

    async _retypeHistoryItem(id) {
        const targetWin = this._capturedWindow;
        if (this._indicator && this._indicator.menu) {
            this._indicator.menu.close();
        }

        GLib.timeout_add(GLib.PRIORITY_DEFAULT, 100, () => {
            if (targetWin && typeof targetWin.activate === 'function') {
                targetWin.activate(global.get_current_time());
            }
            GLib.timeout_add(GLib.PRIORITY_DEFAULT, 150, async () => {
                const res = await runVoxi(['history', 'retype', id]);
                if (!res.success) {
                    try {
                        Main.notify('Voxi Voice Input', `Retype failed: ${res.stderr}`);
                    } catch (e) {}
                }
                return GLib.SOURCE_REMOVE;
            });
            return GLib.SOURCE_REMOVE;
        });
    }

    async _clearHistory() {
        const res = await runVoxi(['history', 'clear']);
        if (res.success) {
            await this._refreshHistory();
        }
    }
}
