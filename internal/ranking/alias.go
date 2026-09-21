package ranking

import "strings"

var stackAliases = map[string]string{
	// JavaScript
	"js":         "javascript",
	"javascript": "javascript",
	"ecmascript": "javascript",

	// TypeScript
	"ts":         "typescript",
	"typescript": "typescript",

	// Node.js
	"node":     "node.js",
	"nodejs":   "node.js",
	"node js":  "node.js",
	"node.js":  "node.js",

	// React
	"react":    "react",
	"reactjs":  "react",
	"react js": "react",
	"react.js": "react",

	// Next.js
	"next":     "next.js",
	"nextjs":   "next.js",
	"next js":  "next.js",
	"next.js":  "next.js",

	// Vue
	"vue":      "vue.js",
	"vuejs":    "vue.js",
	"vue js":   "vue.js",
	"vue.js":   "vue.js",

	// Nuxt
	"nuxt":     "nuxt.js",
	"nuxtjs":   "nuxt.js",
	"nuxt js":  "nuxt.js",
	"nuxt.js":  "nuxt.js",

	// Angular
	"angular":   "angular",
	"angularjs": "angular",

	// Svelte
	"svelte":      "svelte",
	"sveltekit":   "sveltekit",
	"svelte kit":  "sveltekit",

	// Go
	"go":      "golang",
	"golang":  "golang",

	// Python
	"py":      "python",
	"python":  "python",

	// PostgreSQL
	"postgres":   "postgresql",
	"postgresql": "postgresql",
	"postgre":    "postgresql",

	// MySQL
	"mysql":   "mysql",
	"mariadb": "mariadb",

	// MongoDB
	"mongo":   "mongodb",
	"mongodb": "mongodb",

	// SQL Server
	"mssql":      "sql server",
	"sqlserver":  "sql server",
	"sql server": "sql server",

	// Redis
	"redis":   "redis",
	"redisdb": "redis",

	// AWS
	"amazon web services": "aws",
	"aws":                 "aws",

	// Google Cloud
	"gcp":                   "google cloud",
	"google cloud":          "google cloud",
	"google cloud platform": "google cloud",

	// Azure
	"azure":           "azure",
	"microsoft azure": "azure",

	// Kubernetes
	"k8s":        "kubernetes",
	"kube":       "kubernetes",
	"kubernetes": "kubernetes",

	// Docker
	"docker":    "docker",
	"docker.io": "docker",

	// GitHub Actions
	"gha":            "github actions",
	"github actions": "github actions",

	// GitLab CI
	"gitlab-ci": "gitlab ci",
	"gitlab ci": "gitlab ci",

	// Terraform
	"tf":        "terraform",
	"terraform": "terraform",

	// Elasticsearch
	"elastic":       "elasticsearch",
	"elasticsearch": "elasticsearch",

	// RabbitMQ
	"rabbit":   "rabbitmq",
	"rabbitmq": "rabbitmq",

	// Kafka
	"apache kafka": "kafka",
	"kafka":        "kafka",

	// GraphQL
	"graphql": "graphql",

	// REST
	"rest api":    "rest",
	"restful":     "rest",
	"restful api": "rest",

	// .NET
	".net":   ".net",
	"dotnet": ".net",

	// ASP.NET
	"asp.net": "asp.net",
	"aspnet":  "asp.net",

	// C#
	"c#":      "c#",
	"c sharp": "c#",

	// C++
	"cpp":         "c++",
	"c++":         "c++",
	"c plus plus": "c++",

	// Flutter
	"flutter": "flutter",

	// React Native
	"react-native": "react native",
	"react native": "react native",

	// Tailwind
	"tailwind":    "tailwind css",
	"tailwindcss": "tailwind css",

	// Bootstrap
	"bootstrap": "bootstrap",

	// Laravel
	"laravel": "laravel",

	// Django
	"django": "django",

	// FastAPI
	"fastapi":  "fastapi",
	"fast api": "fastapi",

	// Flask
	"flask": "flask",

	// Spring Boot
	"springboot": "spring boot",
	"spring boot": "spring boot",

	// Supabase
	"supabase": "supabase",

	// Firebase
	"firebase": "firebase",
}

func canonicalStackName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))

	n = strings.ReplaceAll(n, "_", " ")
	n = strings.ReplaceAll(n, "-", " ")
	n = strings.Join(strings.Fields(n), " ")

	if canon, ok := stackAliases[n]; ok {
		return canon
	}

	return n
}

func StackNamesMatch(a, b string) bool {
	canonA := canonicalStackName(a)
	canonB := canonicalStackName(b)

	if canonA == canonB {
		return true
	}

	return wholeWordContains(canonA, canonB) ||
		wholeWordContains(canonB, canonA)
}

// wholeWordContains reports whether short appears as a whole word
// inside long.
func wholeWordContains(long, short string) bool {
	if short == "" || long == short {
		return false
	}

	idx := strings.Index(long, short)

	for idx != -1 {
		end := idx + len(short)

		startOK := idx == 0 || long[idx-1] == ' '
		endOK := end == len(long) || long[end] == ' '

		if startOK && endOK {
			return true
		}

		next := strings.Index(long[idx+1:], short)
		if next == -1 {
			break
		}

		idx = idx + 1 + next
	}

	return false
}