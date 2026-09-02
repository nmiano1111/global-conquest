import { request } from "./client";

export type PlayerCountRecord = {
  player_count: number;
  wins: number;
  losses: number;
};

export type LeaderboardEntry = {
  user_id: string;
  username: string;
  wins: number;
  losses: number;
  games_played: number;
  by_player_count: PlayerCountRecord[] | null;
};

export async function getLeaderboard(): Promise<LeaderboardEntry[]> {
  return request<LeaderboardEntry[]>({ method: "GET", url: "/leaderboard" });
}
