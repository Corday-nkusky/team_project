// Validate the SSE contract while bytes arrive so meta does not count as first text.
async function* frames(body) {
  let pending = "";
  for await (const chunk of body.pipeThrough(new TextDecoderStream("utf-8", { fatal: true }))) {
    pending += chunk;
    let boundary;
    while ((boundary = /\r?\n\r?\n/.exec(pending))) {
      const lines = pending.slice(0, boundary.index).split(/\r?\n/);
      pending = pending.slice(boundary.index + boundary[0].length);
      const data = lines.filter((line) => line.startsWith("data:"));
      if (!data.length) continue; // SSE comments / keep-alives
      const event = lines.find((line) => line.startsWith("event:"))?.slice(6).trim();
      yield { event, data: JSON.parse(data.map((line) => line.slice(5)).join("\n")) };
    }
  }
  if (pending.trim()) throw new Error("incomplete_frame");
}

export async function readAskStream(response, started) {
  if (response.headers.get("content-type")?.split(";")[0].trim() !== "text/event-stream") {
    await response.body?.cancel();
    throw new Error("expected_sse");
  }
  let outcome;
  let firstTextMs;
  let doneMs;
  let refusalText = "";
  let answerText = "";
  let metadata;
  const refusal = "资料中未找到相关内容";
  for await (const { event, data } of frames(response.body)) {
    if (doneMs !== undefined) throw new Error("event_after_done");
    if (event === "error") throw new Error("stream_error");
    if (event === "meta") {
      if (outcome || typeof data.no_evidence !== "boolean" || !Array.isArray(data.citations)
          || data.no_evidence !== (data.citations.length === 0)) {
        throw new Error("invalid_meta");
      }
      outcome = data.no_evidence ? "no_evidence" : "answered";
      metadata = data;
      continue;
    }
    if (!outcome) throw new Error("missing_meta");
    if (event === "delta") {
      if (typeof data.text !== "string") throw new Error("invalid_delta");
      if (data.text.trim()) firstTextMs ??= performance.now() - started;
      if (outcome === "no_evidence") {
        refusalText += data.text;
        if (!refusal.startsWith(refusalText)) throw new Error("invalid_refusal");
      } else {
        answerText += data.text;
      }
    } else if (event === "done") {
      if (firstTextMs === undefined) throw new Error("empty_answer");
      if (outcome === "no_evidence" && refusalText !== refusal) throw new Error("invalid_refusal");
      doneMs = performance.now() - started;
    }
  }
  if (doneMs === undefined) throw new Error("missing_done");
  return {
    outcome, firstTextMs, doneMs, answer: outcome === "answered" ? answerText : refusalText,
    citations: metadata.citations, promptVersion: metadata.prompt_version || "",
  };
}
