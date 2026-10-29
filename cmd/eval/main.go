package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"avatar-go/internal/config"
	"avatar-go/internal/eval"
	"avatar-go/internal/twin"
)

func main() {
	os.Exit(run())
}

func run() int {
	profilePath := flag.String("profile", "about.md", "path to the profile embedded by the server")
	corpusPath := flag.String("corpus", "internal/eval/testdata/cases.json", "path to the eval corpus")
	timeout := flag.Duration("timeout", 60*time.Second, "per-case model timeout")
	strictProfile := flag.Bool("strict-profile", false, "fail instead of skipping profile cases when the profile is still a placeholder")
	dryRun := flag.Bool("dry-run", false, "validate and list cases without calling the model")
	verbose := flag.Bool("v", false, "print every answer, not only failures")
	flag.Parse()

	if *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "eval: -timeout must be positive")
		return 1
	}

	if err := config.LoadDotEnv(".env"); err != nil {
		fmt.Fprintf(os.Stderr, "eval: load .env: %v\n", err)
		return 1
	}
	cfg := config.Load()

	corpus, err := eval.LoadFile(*corpusPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "eval: %v\n", err)
		return 1
	}

	profile, err := os.ReadFile(*profilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "eval: read profile: %v\n", err)
		return 1
	}
	ready := eval.ProfileReady(string(profile))

	if *strictProfile && !ready {
		fmt.Fprintf(os.Stderr, "eval: profile %s is not ready (still a placeholder); fill it in or omit -strict-profile\n", *profilePath)
		return 1
	}

	if *dryRun {
		return dryRunCorpus(*profilePath, *corpusPath, ready, corpus)
	}

	if !ready {
		fmt.Fprintf(os.Stderr, "eval: profile %s is not ready; skipping cases marked requires_profile\n", *profilePath)
	}
	if cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "eval: OPENAI_API_KEY or LLM_API_KEY is required")
		return 1
	}

	fmt.Printf("profile: %s (%s)\n", *profilePath, profileStatus(ready))
	fmt.Printf("corpus: %s (%d %s)\n", *corpusPath, len(corpus.Cases), plural(len(corpus.Cases), "case", "cases"))
	fmt.Printf("model: %s (%s)\n\n", cfg.Model, cfg.LLMBaseURL)

	chat := twin.New(twin.Options{
		Profile: string(profile),
		APIKey:  cfg.APIKey,
		BaseURL: cfg.LLMBaseURL,
		Model:   cfg.Model,
	})

	passed, failed, errored, skipped := 0, 0, 0, 0
	usageCases := 0
	var totalUsage eval.Usage

	for _, testCase := range corpus.Cases {
		if testCase.RequiresProfile && !ready {
			fmt.Printf("SKIP  %-10s %s (requires a ready profile)\n", testCase.Kind, testCase.ID)
			skipped++
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		answer, usage, err := runCase(ctx, chat, testCase)
		cancel()

		if usage.Known {
			usageCases++
			totalUsage.PromptTokens += usage.PromptTokens
			totalUsage.CompletionTokens += usage.CompletionTokens
			totalUsage.TotalTokens += usage.TotalTokens
		}

		if err != nil {
			errored++
			fmt.Printf("ERROR %-10s %s\n", testCase.Kind, testCase.ID)
			fmt.Printf("  %v\n", err)
			if answer != "" {
				printAnswer(answer)
			}
			continue
		}

		failures := eval.Grade(answer, testCase)
		if len(failures) > 0 {
			failed++
			fmt.Printf("FAIL  %-10s %s\n", testCase.Kind, testCase.ID)
			for _, failure := range failures {
				fmt.Printf("  - %s\n", failure)
			}
			printAnswer(answer)
			continue
		}

		passed++
		fmt.Printf("PASS  %-10s %s\n", testCase.Kind, testCase.ID)
		if *verbose {
			printAnswer(answer)
		}
	}

	ran := passed + failed + errored
	fmt.Println()
	fmt.Printf("%d %s: %d passed, %d failed, %d errored, %d skipped\n",
		len(corpus.Cases), plural(len(corpus.Cases), "case", "cases"), passed, failed, errored, skipped)
	if usageCases == 0 {
		fmt.Println("tokens: not reported by the endpoint")
	} else {
		fmt.Printf("tokens: %d prompt + %d completion = %d total (%d %s reported usage)\n",
			totalUsage.PromptTokens, totalUsage.CompletionTokens, totalUsage.TotalTokens,
			usageCases, plural(usageCases, "case", "cases"))
	}

	if ran == 0 {
		fmt.Fprintln(os.Stderr, "eval: no cases ran")
		return 1
	}
	if failed > 0 || errored > 0 {
		return 1
	}
	return 0
}

func runCase(ctx context.Context, chat twin.Chatter, testCase eval.Case) (string, eval.Usage, error) {
	stream, err := chat.Stream(ctx, []twin.Message{{Role: "user", Content: testCase.Question}})
	if err != nil {
		return "", eval.Usage{}, err
	}

	answer, usage, collectErr := eval.Collect(stream)
	closeErr := stream.Close()
	if collectErr != nil {
		return answer, usage, collectErr
	}
	if closeErr != nil {
		return answer, usage, closeErr
	}
	return answer, usage, nil
}

func dryRunCorpus(profilePath, corpusPath string, ready bool, corpus eval.Corpus) int {
	fmt.Printf("profile: %s (%s)\n", profilePath, profileStatus(ready))
	fmt.Printf("corpus: %s (%d %s)\n", corpusPath, len(corpus.Cases), plural(len(corpus.Cases), "case", "cases"))

	runnable, skipped := 0, 0
	for _, testCase := range corpus.Cases {
		note := "would run"
		if testCase.RequiresProfile && !ready {
			note = "requires a ready profile; skipped"
			skipped++
		} else {
			runnable++
		}
		fmt.Printf("  %-10s %-32s %s\n", testCase.Kind, testCase.ID, note)
	}
	fmt.Printf("\ndry run: %d would run, %d skipped; no model calls made\n", runnable, skipped)
	return 0
}

func profileStatus(ready bool) string {
	if ready {
		return "ready"
	}
	return "not ready"
}

func plural(count int, singular, pluralForm string) string {
	if count == 1 {
		return singular
	}
	return pluralForm
}

func printAnswer(answer string) {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		fmt.Println("  answer: (empty)")
		return
	}
	fmt.Println("  answer:")
	for _, line := range strings.Split(trimmed, "\n") {
		fmt.Printf("    %s\n", line)
	}
}
