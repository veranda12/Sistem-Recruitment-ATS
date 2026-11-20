import { ApiError, api, type Schemas } from "../../lib/api";
import type { Ack, Batch, ConflictItem, SendResult } from "./autosave";

// Tipe diturunkan dari contracts/openapi.yaml; perubahan kontrak memecah typecheck di sini.
export type AttemptDTO = Schemas["Attempt"];
export type ExamItem = AttemptDTO["items"][number];
export type Receipt = Schemas["Receipt"];

/** Memetakan respons HTTP ke hasil antrean autosave. 429/5xx/jaringan = coba lagi dengan payload sama. */
export function makeSender(attemptId: string, onAck: (a: Ack) => void) {
  return async (batch: Batch): Promise<SendResult> => {
    try {
      const ack = await api<Ack>("PUT", `/exam-attempts/${attemptId}/answers`, batch);
      onAck(ack);
      return { kind: "ack", ack };
    } catch (e) {
      if (!(e instanceof ApiError)) return { kind: "retry" };
      if (e.status === 0 || e.status >= 500 || e.status === 429) return { kind: "retry", retryAfterMs: e.retryAfter ? e.retryAfter * 1000 : undefined };
      if (e.code === "answer_conflict") return { kind: "conflict", items: ((e.details as { items: ConflictItem[] }).items ?? []) };
      if (e.code === "attempt_finalized" || e.code === "deadline_passed") return { kind: "final", code: e.code, message: e.message };
      return { kind: "invalid", message: e.message };
    }
  };
}
