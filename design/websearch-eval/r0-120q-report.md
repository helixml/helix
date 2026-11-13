# Web search relevance: helix vs exa

Generated 2026-11-13T12:23:54Z. Judge `glm-5.3-flash`, prompt `f09afd17e5be`, 120 queries, 1621 new judgments this run.

## Overall

| Metric | helix | exa | Gap |
|---|---:|---:|---:|
| nDCG@10 | 0.676 | 0.814 | -0.138 |
| Mean grade @5 (0-3) | 2.21 | 2.47 | -0.26 |
| Mean grade @10 (0-3) | 2.06 | 2.36 | -0.30 |
| P@3 (grade >= 2) | 80.8% | 84.2% | -3.3% |
| Hit@3 (grade 3 in top 3) | 80.8% | 95.8% | -15.0% |
| Zero-result rate | 2.5% | 0.0% | 2.5% |
| Fetch-failure rate | 12.9% | 0.0% | 12.9% |
| Latency p50 (ms) | 3396 | 1233 | 2163 |
| Latency p95 (ms) | 11201 | 2067 | 9134 |
| Mean cost ($) | 0.0154 | 0.0070 | 0.0084 |

## By query kind

nDCG@10 / mean grade@5 / hit@3, helix vs exa.

| Kind | n | nDCG@10 | Mean@5 | Hit@3 | Zero results | Fetch failures |
|---|---:|---|---|---|---|---|
| company-people | 20 | 0.873 vs 0.427 | 2.22 vs 1.19 | 85% vs 80% | 0% vs 0% | 18% vs 0% |
| descriptive | 20 | 0.440 vs 0.956 | 1.71 vs 2.89 | 60% vs 100% | 10% vs 0% | 8% vs 0% |
| factual | 20 | 0.565 vs 0.855 | 2.02 vs 2.59 | 70% vs 95% | 5% vs 0% | 18% vs 0% |
| news | 15 | 0.503 vs 0.897 | 1.93 vs 2.76 | 73% vs 100% | 0% vs 0% | 9% vs 0% |
| non-english | 20 | 0.745 vs 0.812 | 2.45 vs 2.49 | 90% vs 100% | 0% vs 0% | 10% vs 0% |
| technical | 25 | 0.844 vs 0.930 | 2.73 vs 2.89 | 100% vs 100% | 0% vs 0% | 13% vs 0% |

## Per query

Sorted by helix nDCG@10 gap to exa, worst first. Grades are listed in rank order (`-` = no page text).

| Query | Kind | nDCG@10 | helix grades | exa grades |
|---|---|---|---|---|
| `desc-05` research papers on benchmarks for evaluating retrieval-augmented generation | descriptive | 0.00 vs 1.00 | error | 3 3 3 3 3 3 3 3 3 3 |
| `desc-06` essays about the decline of the open web and SEO spam in search results | descriptive | 0.00 vs 1.00 | error | 3 3 3 3 3 3 3 2 3 2 |
| `news-14` Tesla robotaxi rollout | news | 0.00 vs 0.99 | 0 0 0 0 0- 0 0 0 0 0- | 3 3 3 3 3 2 3 3 3 3 |
| `fact-03` who designed the Go programming language gopher mascot | factual | 0.00 vs 0.99 | error | 3 3 3 3 3 1 3 2 2 3 |
| `desc-19` newsletters covering developer tooling and infrastructure | descriptive | 0.11 vs 0.99 | 1 1 1 0 1 0 1- 1 0 1 | 3 3 3 3 3 3 2 3 3 3 |
| `desc-16` podcasts about the history of computing | descriptive | 0.12 vs 1.00 | 1 1 1 1 1 0 0 1 0 1 | 3 3 3 3 3 3 3 2 3 2 |
| `intl-es2` cómo preparar una entrevista técnica de programación | non-english | 0.13 vs 1.00 | 2 0- 0 0 1 0 0 1 0 1 | 3 3 3 3 3 3 3 3 3 3 |
| `fact-12` Go language release date of version 1.0 | factual | 0.10 vs 0.96 | 1 1 1 0 0- 0 0 0- 1 0 | 3 3 3 3 3 3 0 0 1 0 |
| `desc-20` post-mortems about incidents caused by DNS | descriptive | 0.16 vs 1.00 | 1 1 1 1 1 1 2 1 1 1 | 3 3 3 3 3 3 3 3 3 3 |
| `news-04` major cloud provider outage postmortem | news | 0.20 vs 0.98 | 2 1 2 1 1 | 3 3 3 3 3 3 3 3 2 1 |
| `fact-04` maximum request payload size for a synchronous AWS Lambda invocation | factual | 0.20 vs 0.91 | 1 2 0 2 1 1 1 1 1 0 | 3 1 3 3 3 3 3 3 3 2 |
| `desc-08` case studies of companies moving from AWS to their own bare metal servers | descriptive | 0.35 vs 1.00 | 3 2 1 1 0 1 1 1 1 0 | 3 3 3 3 3 3 3 3 3 3 |
| `news-10` Rust language foundation news | news | 0.27 vs 0.90 | 3 1 | 3 3 3 3 1 2 2 2 3 3 |
| `news-15` SpaceX Starship test flight results | news | 0.32 vs 0.96 | 1 1 2- 3 2 2 2 1 1 1 | 3 3 3 3 3 2 3 3 3 3 |
| `news-01` Kubernetes new release announcement | news | 0.33 vs 0.91 | 3 1 1 0 1 1 1 1 | 3 3 3 3 1 3 2 1 3 3 |
| `intl-ja2` Docker コンテナ 本番環境 ベストプラクティス | non-english | 0.35 vs 0.93 | 2 2 2 1 1 2 1 1 1 1- | 3 3 2 3 1 3 3 2 3 2 |
| `desc-13` research labs working on mechanistic interpretability | descriptive | 0.36 vs 0.93 | 3 1- 2 1 1 1 1 1 1 1 | 3 3 3 3 3 1 3 3 3 3 |
| `desc-18` tools for self-hosting LLMs on a single GPU | descriptive | 0.44 vs 0.99 | 2 2 2 2 1 2 2 2 2 2 | 3 3 3 3 2 3 3 3 2 3 |
| `fact-17` what is the default TTL for an AWS Route 53 record | factual | 0.25 vs 0.80 | 1 1- 0 1 1 1 1- 1 1 1 | 2 1 1 3 2 2 2 2 2 2 |
| `desc-09` companies building AI coding agents for enterprises | descriptive | 0.33 vs 0.87 | 1 2 1 1 1 1 1- 1 1 1 | 2 3 2 1 2 2 2 2 2 2 |
| `desc-10` essays about the craft of writing good technical documentation | descriptive | 0.47 vs 1.00 | 2 2 3 1 1 1 3 3 0 2 | 3 3 3 3 3 3 3 3 3 3 |
| `fact-14` speed of light in vacuum meters per second exact value | factual | 0.48 vs 1.00 | 1 3- 3 3 1 2 1 1 1 2- | 3 3 3 3 3 3 3 3 3 3 |
| `news-09` data breach disclosed by a major retailer | news | 0.48 vs 1.00 | 3 3- 3 1 | 3 3 3 3 3 3 3 3 3 3 |
| `intl-es` buenas prácticas de seguridad para APIs REST | non-english | 0.47 vs 0.93 | 1 3 2 2- 2 2 2 3 1 2 | 3 3 3 3 3 3 3 0 3 3 |
| `desc-17` case studies of companies migrating off Kubernetes | descriptive | 0.37 vs 0.83 | 1 2- 1 3 2 1- 3 1 2 1 | 3 1 3 2 3 3 3 3 3 3 |
| `desc-03` personal blogs of engineers writing about building a compiler from scratch | descriptive | 0.55 vs 1.00 | 1 3 1 1- 3 2 2 3 3 3 | 3 3 3 3 3 3 3 3 3 3 |
| `fact-16` which company created the Kotlin programming language | factual | 0.56 vs 1.00 | 3 3 1 2 2- 1 2- 2 2 1 | 3 3 3 3 3 3 3 3 3 3 |
| `desc-02` blog posts arguing against microservices | descriptive | 0.57 vs 1.00 | 3 1 2 3 2 2 3 1 1 2 | 3 3 3 3 3 3 3 3 3 3 |
| `news-03` Nvidia quarterly earnings results data center revenue | news | 0.41 vs 0.81 | 2 2 2 2 1 1 1 1 3 1 | 2 3 3 1 3 3 3 3 2 2 |
| `intl-lt2` kaip pasirinkti saulės elektrinę namams | non-english | 0.56 vs 0.96 | 2 3 2 1- 1 3 3 2 1 3 | 3 3 3 3 3 3 3 3 2 3 |
| `news-07` Apple WWDC announcements developer tools | news | 0.62 vs 1.00 | 2 2 3 3 3 2 3 1 2 2 | 3 3 3 3 3 3 3 3 3 3 |
| `tech-23` OAuth2 PKCE flow single page application | technical | 0.62 vs 1.00 | 3 3 1 1- 3 2 1 2 1 3 | 3 3 3 3 3 3 3 3 3 3 |
| `desc-01` startups building open-source vector databases | descriptive | 0.47 vs 0.84 | 3 3 1 1 1- 2- 1 1 1 1 | 2 3 3 3 3 3 3 3 2 3 |
| `tech-19` git rebase onto another branch interactive squash commits | technical | 0.64 vs 1.00 | 3 2- 2 3 2 2- 2 2 2 3 | 3 3 3 3 3 3 3 3 3 3 |
| `desc-11` personal blogs of engineers writing about distributed systems | descriptive | 0.52 vs 0.85 | 3 1 3 1- 2 3 1 1 1 1 | 3 3 3 3 3 1 2 2 3 3 |
| `tech-22` TLS certificate chain incomplete error openssl verify | technical | 0.70 vs 1.00 | 3 3 2 3 1 2 2 2 3 2 | 3 3 3 3 3 3 3 3 3 3 |
| `ent-19` Chris Lattner, creator of LLVM and Swift | company-people | 0.49 vs 0.77 | 2 0 2 1 0 1 1 1 0 1- | 3 1 1 1 1 0 1 1 1 0 |
| `news-05` OpenAI new model launch | news | 0.72 vs 1.00 | 1 3 3 3- 3 3 3 3 1 2 | 3 3 3 3 3 3 3 3 3 3 |
| `desc-07` open-source tools for running large language models locally on a laptop | descriptive | 0.65 vs 0.93 | 2 3 3 2 2 2 2 3 2 3 | 3 3 3 3 3 3 3 3 2 2 |
| `fact-07` default port number for Elasticsearch HTTP API | factual | 0.43 vs 0.69 | 2- 2 1 2 1 3 1 1 2 2 | 1 3 1 3 3 3 1 2 3 3 |
| `tech-05` react useEffect cleanup abort fetch request with AbortController | technical | 0.73 vs 1.00 | 3 2- 3- 3- 2- 2 3 2 3 1 | 3 3 3 3 3 3 3 3 3 3 |
| `news-13` Microsoft Azure outage | news | 0.55 vs 0.81 | 1 2 3 2 2 2 3 3 1 2 | 3 3 2 2 3 2 2 2 3 2 |
| `desc-14` tutorials explaining how transformers work with visual diagrams | descriptive | 0.75 vs 1.00 | 3 3 3 3 3 0 1 3 1- 1 | 3 3 3 3 3 3 3 3 3 3 |
| `fact-13` maximum number of pods per node in Kubernetes default | factual | 0.57 vs 0.81 | 2 3 2 2- 1 3 2 2 2 2 | 3 3 3 1 3 1 3 1 2 3 |
| `fact-15` how long is the Linux kernel long-term support period | factual | 0.53 vs 0.77 | 2 2 1 2 3 3 1 3 2 1 | 3 3 3 0 2 2 3 2 1 3 |
| `intl-pt` como funciona a reforma tributária no Brasil | non-english | 0.68 vs 0.90 | 3 3 3 2 2 2 2 2 2 1 | 3 3 2 3 3 3 3 3 3 2 |
| `fact-19` how many parameters does the original GPT-3 model have | factual | 0.54 vs 0.76 | 3 1 3 2 1 2 2 2- 2 1 | 2 1 3 3 3 3 3 3 3 3 |
| `news-02` EU AI Act enforcement and fines | news | 0.68 vs 0.90 | 3 3 1 3 3 1 2 2 2 2 | 3 3 2 3 3 3 3 2 3 3 |
| `desc-15` startups making carbon removal technology | descriptive | 0.70 vs 0.91 | 2 2 3 3 3 3 2 3 1 3 | 3 3 3 2 3 3 3 3 3 2 |
| `fact-18` capital city of Australia and when it was chosen | factual | 0.73 vs 0.93 | 3 2 3 3 3- 2 2- 0 3 2- | 3 3 3 3 3 3 3 3 2 2 |
| `fact-10` how many bits in an IPv6 address | factual | 0.70 vs 0.88 | 3 2 3 2- 2 3- 2- 2 2- 3 | 3 2 3 3 3 3 3 2 3 3 |
| `tech-25` systemd service restart on failure with backoff | technical | 0.78 vs 0.96 | 3 2 3 3 3- 2 3- 2 1- 3 | 3 3 3 3 3 3 3 2 3 3 |
| `fact-09` who invented the World Wide Web and when | factual | 0.82 vs 1.00 | 3- 3 3 2 2 3 3- 2 2 3 | 3 3 3 3 3 3 3 3 3 3 |
| `tech-09` python asyncio.gather vs TaskGroup differences in error handling | technical | 0.64 vs 0.81 | 2 3 3 1 2 3 2 2 2 2 | 3 2 3 2 3 2 3 2 3 2 |
| `tech-24` SQL window functions running total per group | technical | 0.79 vs 0.96 | 2 2 3 3 3 3 3 3 3 3- | 3 3 3 3 3 3 3 3 3 2 |
| `intl-zh` 大语言模型 微调 入门 教程 | non-english | 0.80 vs 0.96 | 3 3 2 3 3 2- 3- 2 3 1 | 3 3 3 3 3 3 3 2 3 3 |
| `intl-it` ricetta tradizionale del ragù alla bolognese | non-english | 0.84 vs 1.00 | 3 3 2 2 3 3 3- 2 3 3- | 3 3 3 3 3 3 3 3 3 3 |
| `intl-pl` jak skonfigurować automatyczne kopie zapasowe PostgreSQL | non-english | 0.49 vs 0.64 | 2 2 3 1 1 1 1 1 2 1 | 3 0 0 0 2 3 3 2 2 0 |
| `news-08` GitHub Copilot new features announced | news | 0.62 vs 0.76 | 3 2 1 3 2- 1 3 3 1 2 | 3 3 3 1 3 1 2 3 3 1 |
| `intl-ja` Pythonで非同期処理を実装する方法 asyncio | non-english | 0.87 vs 1.00 | 3 3 3 3 2 2 3 2 3 3 | 3 3 3 3 3 3 3 3 3 3 |
| `fact-08` maximum size of a single DynamoDB item | factual | 0.75 vs 0.87 | 3 3 3 3- 1 2 2 3 1 2 | 3 3 3 2 3 3 2 3 3 2 |
| `desc-12` open-source alternatives to Datadog for observability | descriptive | 0.88 vs 1.00 | 3 3 3 3 3- 3 2 2 2 3 | 3 3 3 3 3 3 3 3 3 3 |
| `fact-01` default value of max_connections in PostgreSQL | factual | 0.64 vs 0.75 | 3 1- 2 2 3- 3 1 2 1 2- | 3 1 2 3 3 3 0 2 2 3 |
| `fact-05` year the first transatlantic telegraph cable was completed | factual | 0.85 vs 0.96 | 3 3 1- 3 3 3 3 3 3 1 | 3 3 3 3 3 3 3 3 2 3 |
| `fact-02` boiling point of water at the summit of Mount Everest | factual | 0.67 vs 0.77 | 2- 2 3 3 2 2- 3- 3 3 2- | 3 2 2 3 0 3 3 3 3 3 |
| `fact-06` what year was the first version of PostgreSQL released | factual | 0.64 vs 0.75 | 3 3 2 1 2 1 1 2- 1 2 | 2 3 3 0 2 3 3 3 2 0 |
| `tech-06` rust error cannot borrow as mutable more than once at a time fix | technical | 0.80 vs 0.90 | 3- 3 2 3 2 2 2 3 3 3 | 3 3 2 3 3 3 3 2 3 3 |
| `intl-de2` Unterschied zwischen Mietvertrag befristet und unbefristet | non-english | 0.70 vs 0.79 | 3 2 2 3 3 2 2- 2 3 2 | 3 2 3 3 3 3 2 3 1 2 |
| `news-11` Linux kernel release new features | news | 0.50 vs 0.59 | 1 2 3- 2- 3 3 1 1 2 2 | 1 1 3 3 3 1 3 3 1 3 |
| `intl-lt` kaip įregistruoti individualią veiklą Lietuvoje | non-english | 0.79 vs 0.88 | 3- 2 3 3 2 3 2 2 3 3 | 3 3 3 3 3 2 2 3 2 3 |
| `tech-11` postgres partial index vs expression index when to use | technical | 0.74 vs 0.82 | 3 3 2 1 2 2 2 2 1- 2 | 3 2 3 2 2 2 2 2 3 2 |
| `tech-03` kubernetes pod stuck in CrashLoopBackOff how to debug | technical | 0.92 vs 1.00 | 3 3 3 3 3 0 3 3 3- 3 | 3 3 3 3 3 3 3 3 3 3 |
| `tech-12` kubernetes pod stuck in CrashLoopBackOff how to debug | technical | 0.92 vs 1.00 | 3 3 3 3 3 0 3 3- 3 3- | 3 3 3 3 3 3 3 3 3 3 |
| `news-06` ransomware attack on hospital systems | news | 0.84 vs 0.91 | 3 2 3 3 3 3 3 2 2 3 | 3 3 3 3 2 3 3 3 3 2 |
| `tech-04` golang propagate context cancellation to worker goroutines example | technical | 0.80 vs 0.87 | 3 3 3 1 1 3 2 3 3 3 | 3 3 3 3 2 0 3 3 3 3 |
| `tech-01` how to configure nginx rate limiting per client IP | technical | 0.95 vs 1.00 | 3 3 3 3 3 3 3 3 3 1 | 3 3 3 3 3 3 3 3 3 3 |
| `tech-21` redis cache stampede prevention techniques | technical | 0.95 vs 1.00 | 3 3 3 3 3 3 3 3 3 1 | 3 3 3 3 3 3 3 3 3 3 |
| `tech-07` git undo last commit but keep the changes | technical | 0.92 vs 0.96 | 3- 2 3 3 3 3 3 3 3 3 | 3 3 3 3 3 3 3 2 3 3 |
| `tech-17` python asyncio gather vs TaskGroup exception handling | technical | 0.81 vs 0.85 | 3- 3 3 2 2 2- 3 2 3- 3 | 3 3 3 2 2 2 3 3 3 3 |
| `tech-15` react useEffect cleanup function memory leak fetch | technical | 0.92 vs 0.96 | 3 3 3- 3 3 2 3 3 2- 3 | 3 3 3 3 3 2 3 3 3 3 |
| `tech-18` nginx reverse proxy websocket upgrade headers | technical | 0.92 vs 0.95 | 3- 3 3 3 3 2 3 2 3 3 | 3 3 3 3 3 3 3 3 3 1 |
| `tech-02` postgres find and kill long running queries | technical | 0.91 vs 0.92 | 3 3 3 3 3 3 3 3 1- 2 | 3 3 3 3 3 2 3 3 3 2 |
| `desc-04` companies offering self-hosted alternatives to Slack | descriptive | 1.00 vs 1.00 | 3 3 3- 3 3 3 3 3- 3 3- | 3 3 3 3 3 3 3 3 3 3 |
| `tech-08` terraform state lock error with S3 backend and DynamoDB force-unlock | technical | 0.89 vs 0.87 | 3 3 3 2 3- 3- 3 1 3- 3 | 2 3 3 3 3 3 3 3 3 3 |
| `intl-de` Kündigungsfrist Mietwohnung bei Eigenbedarf des Vermieters | non-english | 0.87 vs 0.83 | 3 2 3 2 3 3 3 3- 3 3 | 3 3 3 3 2 2 3 3 2 2 |
| `intl-ru` как настроить мониторинг сервера с Prometheus и Grafana | non-english | 1.00 vs 0.96 | 3 3 3 3 3 3 3 3 3 3 | 3 3 3 3 3 3 3 3 2 3 |
| `fact-20` what does the HTTP status code 418 mean | factual | 1.00 vs 0.96 | 3 3 3 3 3 3 3 3 3 3 | 3 3 3 3 3 3 3 2 3 3 |
| `tech-20` docker multi-stage build reduce image size go binary | technical | 0.91 vs 0.87 | 3 3 3- 3 2 3 3 3 3- 2 | 2 3 3 3 3 3 3 3 3 3 |
| `tech-13` rust borrow checker error cannot borrow as mutable more than once | technical | 0.96 vs 0.91 | 3 3- 3 3 3 3 3 2 3 3 | 3 3 3 3 2 3 3 2 3 3 |
| `tech-14` terraform state lock error dynamodb how to fix | technical | 0.92 vs 0.87 | 3 3 3- 3 3 3 3 2 3 2 | 2 3 3 3 3 3 3 3 3 3 |
| `news-12` semiconductor export controls China chips | news | 1.00 vs 0.94 | 3- 3 3 3 3 3 3- 3 3 3- | 3 3 2 3 3 3 3 3 3 3 |
| `intl-it2` come funziona la fatturazione elettronica in Italia | non-english | 0.89 vs 0.82 | 3 3 3 3 3 1- 2 3 3 3 | 2 3 3 2 3 3 3 3 3 3 |
| `tech-10` docker compose depends_on condition service_healthy healthcheck example | technical | 1.00 vs 0.92 | 3 3 3 3 3 3 3 3 3 3 | 3 3 3 3 3 3 2 3 2 3 |
| `tech-16` golang context cancellation propagate to goroutines | technical | 0.96 vs 0.84 | 3 3 3 3 3- 3 3 3 3 2 | 3 3 3 3 2 1 2 3 3 3 |
| `intl-pt2` diferença entre microsserviços e arquitetura monolítica | non-english | 1.00 vs 0.88 | 3 3 3 3 3 3 3 3 3 3 | 3 3 3 3 3 2 2 3 2 3 |
| `intl-ru2` что такое квантовые вычисления простыми словами | non-english | 0.95 vs 0.81 | 3- 3 3 2 3 3- 3 3 3 3 | 3 3 3 3 3 2 0 2 3 2 |
| `ent-14` Guido van Rossum | company-people | 0.67 vs 0.51 | 2 2- 2 1 3 1 2 1 1 1 | 3 1 1 1 1 0 0 0 0 0 |
| `intl-pl2` najlepsze praktyki bezpieczeństwa haseł w aplikacjach webowych | non-english | 0.71 vs 0.51 | 3 2 2 2- 2- 2 2- 2 2- 1 | 3 0 0 0 3 0 0 3 0 1 |
| `ent-15` Linus Torvalds | company-people | 0.65 vs 0.38 | 1- 1- 2 2 2 3- 2 1 2 2 | 2 2 1 1 0 0 1 0 0 1 |
| `ent-07` Supabase | company-people | 0.90 vs 0.61 | 2 3 2 2- 2 1 2- 2 2- 1 | 3 1 1 1 1 1 1 1 0 0 |
| `fact-11` what is the boiling point of water at the top of Mount Everest | factual | 0.86 vs 0.54 | 3- 3- 3 3 3- 0 3 1 3- 3- | 1 3 2 2 2 2 1 3 3 3 |
| `ent-01` Hugging Face | company-people | 0.95 vs 0.62 | 3 2 3 1 1 1 2 2 1 1 | 3 1 1 1 1 1 1 1 1 1 |
| `intl-fr` comment installer un serveur VPN WireGuard sur Ubuntu | non-english | 0.86 vs 0.48 | 3 3 3 2- 3 3 2 2 3 3 | 3 0 0 3 0 0 3 2 3 0 |
| `ent-12` Fly.io | company-people | 0.96 vs 0.54 | 3 1- 2 2- 2 2- 2 1- 2 1 | 3 1 1 0 0 1 0 0 0 0 |
| `ent-06` Simon Willison, creator of Datasette | company-people | 0.83 vs 0.40 | 3 2 2 2 2 2 1 1 1 2 | 3 1 0 0 0 0 0 0 0 0 |
| `ent-08` Vercel, the frontend cloud platform | company-people | 0.93 vs 0.50 | 3 2- 2 2 2 2 2- 2 3 1 | 3 1 1 1 0 1 1 1 1 0 |
| `intl-fr2` comment configurer un serveur VPN WireGuard sur Ubuntu | non-english | 1.00 vs 0.53 | 3 3 3 3 3 3 3 3 3 3 | 3 0 3 3 0 3 2 0 0 0 |
| `ent-18` Patrick Collison, Stripe co-founder | company-people | 0.84 vs 0.36 | 2 3 3 3 3 3 3 2 1 2 | 3 1 1 1 1 1 0 1 1 1 |
| `ent-09` Anthropic, AI safety company | company-people | 0.97 vs 0.50 | 3 3 2 3- 1 2- 1 1- 2 2 | 3 1 1 0 1 1 1 1 1 1 |
| `ent-13` Temporal, the durable execution platform | company-people | 0.92 vs 0.44 | 3 2- 1 2 2 3 2 2- 2 2 | 3 1 0 0 0 0 1 0 1 1 |
| `ent-16` Kelsey Hightower | company-people | 0.81 vs 0.32 | 2- 3- 1- 3 3 3 0 3- 3 3 | 1 3 2 1 1 1 1 1 0 0 |
| `ent-05` Mitchell Hashimoto, co-founder of HashiCorp | company-people | 0.85 vs 0.34 | 3 2 2- 2 2 3 3 2 3- 3 | 3 1 1 0 1 0 0 1 1 0 |
| `intl-zh2` Kubernetes 和 Docker Swarm 的区别 | non-english | 0.95 vs 0.43 | 3 3- 3 3 2 3 3- 3 3 3 | 3 3 0 0 0 0 0 3 0 0 |
| `ent-03` Mistral AI, French large language model startup | company-people | 0.91 vs 0.39 | 3 2 2 3 3 3 1 2 3 3 | 3 1 1 1 1 1 0 1 1 1 |
| `ent-20` Charity Majors, Honeycomb co-founder | company-people | 0.96 vs 0.42 | 3 3 3- 2 2 3 2 3 0 2- | 3 1 1 1 1 1 1 1 1 1 |
| `ent-02` Tailscale, the WireGuard-based VPN company | company-people | 1.00 vs 0.45 | 3 3 3 3- 2 2 2- 1 2 2 | 3 1 1 1 1 1 1 1 1 1 |
| `ent-04` Andrej Karpathy | company-people | 0.98 vs 0.31 | 3 3 3 3 2 2 3 2 3 2- | 3 1 0 1 0 0 1 0 0 0 |
| `ent-10` Cloudflare Workers | company-people | 0.93 vs 0.25 | 3 2 2 1 1 2 1 1 2 1 | 0 2 1 1 1 0 1 1 0 0 |
| `ent-17` Julia Evans, writer of programming zines | company-people | 0.94 vs 0.22 | 3 3 3 2 2 2 2 2 0 0 | 2 1 1 1 0 0 0 0 0 1 |
| `ent-11` Grafana Labs | company-people | 0.96 vs 0.21 | 3 2 2 2- 2 2- 1- 1- 2 2- | 2 0 1 0 0 0 0 0 0 0 |
