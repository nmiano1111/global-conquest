import { request } from "./client";

export type LeaderboardEntry = {
  user_id: string;
  username: string;
  wins: number;
  losses: number;
  games_played: number;
};

export async function getLeaderboard(): Promise<LeaderboardEntry[]> {
  return request<LeaderboardEntry[]>({ method: "GET", url: "/leaderboard" });
}
