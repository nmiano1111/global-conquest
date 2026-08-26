import { request } from "./client";

type UnknownRecord = Record<string, unknown>;

export type BotPacingConfig = {
  turnStartMinMs: number;
  turnStartMaxMs: number;
  cardTurnInMinMs: number;
  cardTurnInMaxMs: number;
  reinforcementMinMs: number;
  reinforcementMaxMs: number;
  firstAttackMinMs: number;
  firstAttackMaxMs: number;
  repeatAttackMinMs: number;
  repeatAttackMaxMs: number;
  captureMinMs: number;
  captureMaxMs: number;
  occupationMinMs: number;
  occupationMaxMs: number;
  fortifyMinMs: number;
  fortifyMaxMs: number;
  endAttackMinMs: number;
  endAttackMaxMs: number;
  dramaticMinMs: number;
  dramaticMaxMs: number;
};

/** Mirrors backend risk.BotTuning -- the admin-tunable killbot-v2 AI
 * parameters, snapshotted onto every new bot-inclusive game at creation
 * time. Editing this only affects games created afterward. */
export type BotTuningConfig = {
  killAdvantageRatio: number;
  killClusterMaxSize: number;
  minAttackWinProbability: number;
  hogwildArmiesPerTerritory: number;
  pacing: BotPacingConfig;
};

function asRecord(value: unknown): UnknownRecord | null {
  if (!value || typeof value !== "object") return null;
  return value as UnknownRecord;
}

function readNumber(value: unknown, fallback = 0): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

const PACING_FIELDS: [keyof BotPacingConfig, string][] = [
  ["turnStartMinMs", "turn_start_min_ms"],
  ["turnStartMaxMs", "turn_start_max_ms"],
  ["cardTurnInMinMs", "card_turn_in_min_ms"],
  ["cardTurnInMaxMs", "card_turn_in_max_ms"],
  ["reinforcementMinMs", "reinforcement_min_ms"],
  ["reinforcementMaxMs", "reinforcement_max_ms"],
  ["firstAttackMinMs", "first_attack_min_ms"],
  ["firstAttackMaxMs", "first_attack_max_ms"],
  ["repeatAttackMinMs", "repeat_attack_min_ms"],
  ["repeatAttackMaxMs", "repeat_attack_max_ms"],
  ["captureMinMs", "capture_min_ms"],
  ["captureMaxMs", "capture_max_ms"],
  ["occupationMinMs", "occupation_min_ms"],
  ["occupationMaxMs", "occupation_max_ms"],
  ["fortifyMinMs", "fortify_min_ms"],
  ["fortifyMaxMs", "fortify_max_ms"],
  ["endAttackMinMs", "end_attack_min_ms"],
  ["endAttackMaxMs", "end_attack_max_ms"],
  ["dramaticMinMs", "dramatic_min_ms"],
  ["dramaticMaxMs", "dramatic_max_ms"],
];

function normalizePacing(value: unknown): BotPacingConfig {
  const record = asRecord(value) ?? {};
  const out = {} as BotPacingConfig;
  for (const [camel, snake] of PACING_FIELDS) {
    out[camel] = readNumber(record[snake]);
  }
  return out;
}

function normalizeBotTuningConfig(value: unknown): BotTuningConfig {
  const record = asRecord(value);
  if (!record) {
    return {
      killAdvantageRatio: 0,
      killClusterMaxSize: 0,
      minAttackWinProbability: 0,
      hogwildArmiesPerTerritory: 0,
      pacing: normalizePacing(null),
    };
  }
  return {
    killAdvantageRatio: readNumber(record.kill_advantage_ratio),
    killClusterMaxSize: readNumber(record.kill_cluster_max_size),
    minAttackWinProbability: readNumber(record.min_attack_win_probability),
    hogwildArmiesPerTerritory: readNumber(record.hogwild_armies_per_territory),
    pacing: normalizePacing(record.pacing),
  };
}

function serializePacing(pacing: BotPacingConfig): UnknownRecord {
  const out: UnknownRecord = {};
  for (const [camel, snake] of PACING_FIELDS) {
    out[snake] = pacing[camel];
  }
  return out;
}

function serializeBotTuningConfig(cfg: BotTuningConfig): UnknownRecord {
  return {
    kill_advantage_ratio: cfg.killAdvantageRatio,
    kill_cluster_max_size: cfg.killClusterMaxSize,
    min_attack_win_probability: cfg.minAttackWinProbability,
    hogwild_armies_per_territory: cfg.hogwildArmiesPerTerritory,
    pacing: serializePacing(cfg.pacing),
  };
}

export async function getBotTuningConfig(): Promise<BotTuningConfig> {
  const res = await request<unknown>({ method: "GET", url: "/admin/bot-tuning" });
  return normalizeBotTuningConfig(res);
}

export async function updateBotTuningConfig(cfg: BotTuningConfig): Promise<BotTuningConfig> {
  const res = await request<unknown>({
    method: "PUT",
    url: "/admin/bot-tuning",
    data: serializeBotTuningConfig(cfg),
  });
  return normalizeBotTuningConfig(res);
}
