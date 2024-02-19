import React from "react";
import { LaTeXSource } from "./styles";

export type LeaderboardEntry = [
  username: string,
  inGame: boolean,
  score: number
];

const Leaderboard = React.memo(
  (props: { entries: Array<LeaderboardEntry> }) => {
    return (
      <>
        <table style={LaTeXSource}>
          <thead>
            <tr>
              <th>
                <u>Player Name</u>
              </th>
              <th>
                <u>Score</u>
              </th>
            </tr>
          </thead>
          <tbody style={{ textAlign: "center" }}>
            {props.entries.map((x) => (
              <tr>
                <td key={x[0]}>{x[0]}</td>
                <td>{x[2]}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </>
    );
  }
);

export default Leaderboard;
