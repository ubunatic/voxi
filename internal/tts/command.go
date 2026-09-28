package tts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/spec"
)

// NewSayCommand creates the monitor-gated voxi say CLI command.
func NewSayCommand(d deps.Dependencies) *cobra.Command {
	interrupt := false
	from := ""
	llmHost := ""
	disableLLM := false
	output := ""
	noPlay := false
	cmd := &cobra.Command{
		Use:   "say [text...]",
		Short: "Rewrite text for speech with an LLM, then read it aloud",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			home := ""
			if d.Getenv != nil {
				home = d.Getenv("HOME")
			}
			if _, err := config.LoadUserSettings(home); err != nil {
				return fmt.Errorf("load TTS configuration: %w", err)
			}
			if d.Getenv != nil {
				if backend := strings.TrimSpace(d.Getenv("VOXI_TTS_BACKEND")); backend != "" {
					if err := config.ValidateTTSBackend(backend); err != nil {
						return err
					}
				}
			}
			var text string
			if from != "" {
				if len(args) > 0 {
					return fmt.Errorf("--from cannot be combined with text arguments")
				}
				if from != "primary" && from != "clipboard" {
					return fmt.Errorf("--from must be primary or clipboard")
				}
				pasteArgs := []string{}
				if from == "primary" {
					pasteArgs = append(pasteArgs, "--primary")
				}
				if d.LookPath != nil {
					if _, err := d.LookPath("wl-paste"); err != nil {
						return fmt.Errorf("read %s selection: wl-paste is required: %w", from, err)
					}
				}
				var err error
				if d.RunOutput != nil {
					text, err = d.RunOutput(cmd.Context(), "wl-paste", pasteArgs...)
				} else {
					var output []byte
					output, err = exec.CommandContext(cmd.Context(), "wl-paste", pasteArgs...).Output()
					text = string(output)
				}
				if err != nil {
					return fmt.Errorf("read %s selection: %w", from, err)
				}
				if strings.TrimSpace(text) == "" {
					return fmt.Errorf("%s selection is empty", from)
				}
			} else if len(args) > 0 {
				text = strings.Join(args, " ")
			} else {
				data, err := io.ReadAll(io.LimitReader(d.Stdin, maxTextBytes+1))
				if err != nil {
					return fmt.Errorf("read text from stdin: %w", err)
				}
				if len(data) > maxTextBytes {
					return fmt.Errorf("text exceeds %d byte limit", maxTextBytes)
				}
				text = string(data)
			}
			if output != "" || noPlay {
				if output == "" {
					return fmt.Errorf("--no-play requires --output <file.wav>")
				}
				executable, err := os.Executable()
				if err != nil {
					return err
				}
				audio, _, err := NewEngine(d, executable).Synthesize(cmd.Context(), text)
				if err != nil {
					return fmt.Errorf("synthesize WAV: %w", err)
				}
				defer audio.Close()
				input, err := os.Open(audio.path)
				if err != nil {
					return fmt.Errorf("open synthesized WAV: %w", err)
				}
				defer input.Close()
				file, err := os.Create(output)
				if err != nil {
					return fmt.Errorf("create WAV output %s: %w", output, err)
				}
				if _, err := io.Copy(file, input); err != nil {
					_ = file.Close()
					return fmt.Errorf("write WAV output %s: %w", output, err)
				}
				if err := file.Close(); err != nil {
					return fmt.Errorf("close WAV output %s: %w", output, err)
				}
				fmt.Fprintf(d.Stdout, "wrote %s\n", output)
				return nil
			}
			runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
			if d.Getenv != nil {
				runtimeDir = d.Getenv("XDG_RUNTIME_DIR")
			}
			client := Client{SocketPath: SocketPath(runtimeDir, os.Getuid())}
			if disableLLM {
				count, err := queueSay(client, cmd.Context(), text, interrupt)
				if err != nil {
					return err
				}
				fmt.Fprintf(d.Stdout, "queued %d TTS chunk(s)\n", count)
				return nil
			}
			return sayWithLLM(cmd.Context(), d, client, text, interrupt, llmHost)
		},
	}
	cmd.Flags().BoolVar(&interrupt, "interrupt", false, "stop current TTS and replace it with this text")
	cmd.Flags().StringVar(&from, "from", "", "read from Wayland primary selection or clipboard")
	cmd.Flags().StringVar(&llmHost, "llm", "", "override the lmcoder host; narration is enabled by default")
	cmd.Flags().BoolVar(&disableLLM, "no-llm", false, "read the original text without LLM rewriting")
	cmd.Flags().StringVarP(&output, "output", "o", "", "synthesize WAV directly to this file without playback")
	cmd.Flags().BoolVar(&noPlay, "no-play", false, "synthesize without playback (requires --output)")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return cmd
}

const narrationSystemPrompt = "You prepare text for speech synthesis. Preserve every fact and all important detail. Do not invent, omit, summarize, or translate. Render Markdown structure naturally for listening; describe table rows as clear spoken comparisons. Output only the narration, with no preamble or formatting markers."

func queueSay(client Client, ctx context.Context, text string, interrupt bool) (int, error) {
	if interrupt {
		return client.Replace(ctx, text)
	}
	return client.Enqueue(ctx, text)
}

func sayWithLLM(ctx context.Context, d deps.Dependencies, client Client, text string, interrupt bool, hostOverride string) error {
	ttsSpec, err := spec.LoadTTS()
	if err != nil {
		return err
	}
	host := strings.TrimSpace(hostOverride)
	if host == "" {
		home := ""
		if d.Getenv != nil {
			home = d.Getenv("HOME")
		}
		settings, configErr := config.LoadUserSettings(home)
		if configErr != nil {
			return fmt.Errorf("load Voxi configuration: %w", configErr)
		}
		host = strings.TrimSpace(settings.TTSLLMHost)
	}
	if host == "" {
		host = ttsSpec.LLM.DefaultHost
	}
	if d.LookPath != nil {
		if _, err := d.LookPath("lmcoder"); err != nil {
			count, queueErr := queueSay(client, ctx, text, interrupt)
			if queueErr != nil {
				return queueErr
			}
			fmt.Fprintf(d.Stdout, "lmcoder is unavailable; queued original text (%d TTS chunk(s))\n", count)
			return nil
		}
	}
	paragraphs := splitParagraphs(text)
	if len(paragraphs) == 0 {
		return errors.New("text is empty")
	}
	var sessionBytes [16]byte
	if _, err := rand.Read(sessionBytes[:]); err != nil {
		return fmt.Errorf("create isolated lmcoder session: %w", err)
	}
	session := "voxi-say-" + hex.EncodeToString(sessionBytes[:])
	firstPrompt := "Narrate the first paragraph of a document. More text will arrive in a later turn. Read this paragraph naturally and faithfully; do not announce that more is coming.\n\n" + paragraphs[0]
	first, err := runNarration(ctx, d, host, session, ttsSpec.LLM.MaxTokens, firstPrompt)
	if err != nil || strings.TrimSpace(first) == "" {
		count, queueErr := queueSay(client, ctx, text, interrupt)
		if queueErr != nil {
			return queueErr
		}
		fmt.Fprintf(d.Stdout, "LLM narration unavailable; queued original text (%d TTS chunk(s))\n", count)
		return nil
	}
	count, err := queueSay(client, ctx, first, interrupt)
	if err != nil {
		return err
	}
	if len(paragraphs) > 1 {
		rest := strings.Join(paragraphs[1:], "\n\n")
		followUp := "Continue the same document. Use the conversation so far for context and narrate all of the remaining text below. Preserve every fact and detail, and render Markdown structures naturally for listening. Output only the narration.\n\n" + rest
		continued, followErr := runNarration(ctx, d, host, session, ttsSpec.LLM.MaxTokens, followUp)
		if followErr != nil || strings.TrimSpace(continued) == "" {
			queued, queueErr := client.Enqueue(ctx, rest)
			if queueErr != nil {
				return queueErr
			}
			count += queued
			fmt.Fprintf(d.Stdout, "LLM continuation unavailable; queued original remaining text (%d additional TTS chunk(s))\n", queued)
			return nil
		}
		queued, queueErr := client.Enqueue(ctx, continued)
		if queueErr != nil {
			return queueErr
		}
		count += queued
	}
	fmt.Fprintf(d.Stdout, "queued %d LLM narration TTS chunk(s) via %s\n", count, host)
	return nil
}

func runNarration(ctx context.Context, d deps.Dependencies, host, session string, maxTokens int, prompt string) (string, error) {
	args := []string{"prompt", "--host", host, "--session", session, "--raw", "--format", "plain", "--no-preamble", "--max-tokens", strconv.Itoa(maxTokens), "--system", narrationSystemPrompt}
	if d.RunStdinOutput != nil {
		return d.RunStdinOutput(ctx, prompt, "lmcoder", args...)
	}
	cmd := exec.CommandContext(ctx, "lmcoder", args...)
	cmd.Stdin = strings.NewReader(prompt)
	output, err := cmd.Output()
	return string(output), err
}

func splitParagraphs(text string) []string {
	blocks := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n")
	paragraphs := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block) != "" {
			paragraphs = append(paragraphs, strings.TrimSpace(block))
		}
	}
	return paragraphs
}

// ReportCommandError prints errors to stderr and also notifies from terminal-free shortcuts.
func ReportCommandError(d deps.Dependencies, err error) {
	if err == nil {
		return
	}
	if d.Stderr != nil {
		fmt.Fprintln(d.Stderr, err)
	}
	if d.LookPath == nil {
		return
	}
	notify, lookErr := d.LookPath("notify-send")
	if lookErr != nil {
		return
	}
	run := d.Run
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		}
	}
	_ = run(context.Background(), notify, "Voxi", err.Error())
}
