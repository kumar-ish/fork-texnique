import Countdown from "./Countdown";
import { LaTeXButton } from "./styles";

const Scoreboard = ({
  score,
  timeLimit,
  gameOver,
  newProblem,
}: {
  score: number;
  timeLimit?: number;
  gameOver?: () => void;
  newProblem?: () => void;
}) => {
  return (
    <div style={{ display: "flex", justifyContent: "space-between" }}>
      <div>
        <button
          type="button"
          hidden={newProblem == undefined}
          onClick={newProblem}
          style={LaTeXButton}
        >
          Skip This Problem
        </button>
        <button
          type="button"
          hidden={newProblem == undefined}
          onClick={gameOver}
          style={LaTeXButton}
        >
          End Game
        </button>
      </div>
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "5fr 4fr",
          textAlign: "right",
        }}
      >
        <div>
          <b>Score:</b>
        </div>
        <div> {score}</div>{" "}
        <div>
          <b>Time:</b>{" "}
        </div>
        <div>
          <Countdown timeLimit={timeLimit} overCallback={gameOver} />
        </div>
      </div>
    </div>
  );
};

export default Scoreboard;
