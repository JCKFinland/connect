export async function consumeSSE(stream, { onEvent, signal }) {
  const reader = stream.getReader();
  const decoder = new TextDecoder();

  let buffer = "";

  try {
    while (true) {
      if (signal?.aborted) {
        return;
      }

      const { value, done } = await reader.read();

      if (done) {
        break;
      }

      buffer += decoder.decode(value, {
        stream: true,
      });

      buffer = processBuffer(buffer, onEvent);
    }

    buffer += decoder.decode();

    processBuffer(`${buffer}\n\n`, onEvent);
  } finally {
    reader.releaseLock();
  }
}

function processBuffer(buffer, onEvent) {
  const normalized = buffer.replace(/\r\n/g, "\n");

  const blocks = normalized.split("\n\n");

  const remainder = blocks.pop() ?? "";

  for (const block of blocks) {
    processEventBlock(block, onEvent);
  }

  return remainder;
}

function processEventBlock(block, onEvent) {
  if (!block.trim()) {
    return;
  }

  let eventType = "message";
  const dataLines = [];

  for (const line of block.split("\n")) {
    if (line.startsWith(":") || line === "") {
      continue;
    }

    if (line.startsWith("event:")) {
      eventType = line.slice("event:".length).trimStart();

      continue;
    }

    if (line.startsWith("data:")) {
      dataLines.push(line.slice("data:".length).trimStart());
    }
  }

  const rawData = dataLines.join("\n");

  let data = rawData;

  if (rawData) {
    try {
      data = JSON.parse(rawData);
    } catch {
      // Keep non-JSON SSE data as text.
    }
  }

  onEvent?.({
    type: eventType,
    data,
  });
}
