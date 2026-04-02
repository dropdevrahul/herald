package main

import (
	"dropdevrahul/herald/src/model"
	"dropdevrahul/herald/src/model/openai"
	workflows "dropdevrahul/herald/src/worklows"
	"fmt"
	"net/http"
)

func main() {
	n := workflows.Node{
		Prompt: "You are a reasoning engine. Break down the problem into clear logical steps. Be thorough but concise.",
	}
	n2 := workflows.Node{
		Prompt: "Based on the provided steps execute them and return the result.",
	}
	n3 := workflows.Node{
		Prompt: "Based on the provided result generate code snippets that can be used to implement the solution.",
	}
	
	m := openai.NewOpenAIModel(model.ModelOptions{
		Model: "llama-3.3-70b-versatile",
	})
	
	wf := workflows.NewChainingWorkflow(m, []workflows.Node{n, n2, n3})

	http.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		output, err := wf.Run(r.Context(), r.URL.Query().Get("q"))
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
			flusher.Flush()
			return
		}

		w.Write([]byte(output))
		flusher.Flush()
	})

	println("Starting server on :9000")
	http.ListenAndServe("0.0.0.0:9000", nil)
}
