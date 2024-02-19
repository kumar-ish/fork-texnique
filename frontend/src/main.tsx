import { useState, useEffect, useRef, useCallback } from "react";
import ReactDOM from "react-dom/client";
import {
  createBrowserRouter,
  RouterProvider,
  useNavigate,
  useParams,
} from "react-router-dom";
import { useForm, SubmitHandler } from "react-hook-form";
import "./index.css";
import { problems } from "./default_problems";
import katex from "katex";
import html2canvas from "html2canvas";
import pixelmatch from "pixelmatch";
import "./katex.min.css";
import { ClientSent, Problem, ServerSent } from "./message_passing.ts";
import { google } from "./google/protobuf/timestamp.ts";
import Scoreboard from "./Scoreboard.tsx";
import { pickRandom } from "./helper.ts";
import CreateLobby from "./CreateLobby.tsx";
import LoginForm from "./LoginForm.tsx";
import Button from "./components/Button.tsx";
import Leaderboard, { LeaderboardEntry } from "./Leaderboard.tsx";
import { BoxStyle, LaTeXSource } from "./styles.ts";
import "katex";

const KaTeXWrapper = ({ text }: { text: string }) => {
  const ref = useRef<HTMLInputElement>(null);
  const displaySettings = {
    displayMode: true,
    throwOnError: false,
  };

  useEffect(() => {
    katex.render(text, ref.current as HTMLElement, displaySettings);
  });
  return <p ref={ref} />;
};

const validateMatch = (
  goalCanvas: HTMLCanvasElement,
  typedCanvas: HTMLCanvasElement
) => {
  if (
    goalCanvas.width != typedCanvas.width ||
    goalCanvas.height != typedCanvas.height
  ) {
    return false;
  }
  const [height, width] = [goalCanvas.height, goalCanvas.width];
  const [goalImage, typedImage] = [
    goalCanvas.getContext("2d")?.getImageData(0, 0, width, height),
    typedCanvas.getContext("2d")?.getImageData(0, 0, width, height),
  ];
  if (!goalImage || !typedImage) return false;
  const diff = pixelmatch(
    goalImage.data,
    typedImage.data,
    null,
    width,
    height,
    {
      threshold: 0.1,
    }
  );
  console.log(diff);

  return diff == 0;
};

function LatexBoxes({
  goalText,
  correctCallback,
}: {
  goalText: {
    latex: string;
    description: string;
    title: string;
  };
  correctCallback: (arg0?: string) => void;
}) {
  const [typedText, setTypedText] = useState("");
  const typedRef = useRef<HTMLInputElement>(null);
  const goalRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    katex.render(typedText, typedRef.current as HTMLInputElement, {
      throwOnError: false,
    });
  }, [typedText]);
  useEffect(() => {
    katex.render(goalText.latex, goalRef.current as HTMLInputElement, {
      throwOnError: false,
    });
  }, [goalText]);

  const setValidated = async (text: string) => {
    setTypedText(text);
    const valid = await Promise.all([
      html2canvas(goalRef.current as HTMLElement),
      html2canvas(typedRef.current as HTMLElement),
    ]).then((value) => {
      const [goalCanvas, typedCanvas] = value;
      return validateMatch(goalCanvas, typedCanvas);
    });
    if (valid) {
      correctCallback(typedText);
      setTypedText("");
    }
  };

  return (
    <div>
      <header className="App-header">
        <h2>
          <b>{goalText.title}</b>
        </h2>
        <h3>
          <b>{goalText.description}</b>
        </h3>
        <div>Try to create the following formula:</div>
        <div ref={goalRef} style={BoxStyle} />
        <div>This is what your output looks like:</div>
        <div ref={typedRef} style={BoxStyle} />
        <div>Edit your code here:</div>
        <input
          value={typedText}
          style={{ ...LaTeXSource, ...BoxStyle }}
          onChange={(e) => setValidated(e.target.value)}
        />
      </header>
    </div>
  );
}

const SinglePlayerLaTeX = (props: { timeLimit?: number }) => {
  const [score, setScore] = useState(0);
  const [problem, setProblem] = useState(pickRandom(problems));
  const navigate = useNavigate();
  const gameOver = () => {
    navigate("/");
  };
  const newProblem = () => {
    setProblem(pickRandom(problems));
  };

  return (
    <div>
      <Scoreboard
        score={score}
        timeLimit={props.timeLimit}
        gameOver={gameOver}
        newProblem={newProblem}
      />
      <LatexBoxes
        goalText={problem}
        correctCallback={() => {
          setScore(score + problem.latex.length);
          newProblem();
        }}
      />
    </div>
  );
};

type RequestStartParams = {
  duration: number;
  isRandom: boolean;
  problems: string;
};

type GameTime = {
  startTime: Date;
  duration: number;
};

const LobbyMembers = ({ members }: { members: Array<LeaderboardEntry> }) => {
  return (
    <div>
      {members.map((x) => (
        <div key={x[0]}>hi{x[0]}</div>
      ))}
    </div>
  );
};

const Game = () => {
  const { lobbyId } = useParams();
  const [loggedIn, setOtp] = useState({
    otp: localStorage.getItem(lobbyId!),
    is_owner: localStorage.getItem(lobbyId! + "_is_owner") == "true",
  });
  const [members, setMembers] = useState<Array<LeaderboardEntry>>([]);
  const [gameTime, setGameTime] = useState<GameTime>();
  const ws = useRef<WebSocket>();
  const navigate = useNavigate();
  const [score, setScore] = useState(0);
  const [problem, setProblem] = useState<Problem>();

  const gameOver = useCallback(() => {
    navigate("/");
  }, [navigate]);
  const newProblem = useCallback(() => {
    ws.current!.send(
      new ClientSent({
        request_problem: new ClientSent.RequestProblem(),
      }).serialize()
    );
  }, []);
  const submitAnswer = useCallback((answer: string) => {
    ws.current!.send(
      new ClientSent({
        answer: new ClientSent.GiveAnswer({ answer }),
      }).serialize()
    );
  }, []);
  useEffect(() => {
    ws.current = new WebSocket(
      `ws://localhost:8080/ws?otp=${loggedIn.otp}&l=${lobbyId}`
    );
    ws.current.binaryType = "blob";
    ws.current.onopen = () => console.log("ws opened");
    ws.current.onclose = () => console.log("ws closed");

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ws.current.onmessage = async function (evt: MessageEvent<any>) {
      const rawArray = new Uint8Array(await evt.data.arrayBuffer());
      const event = ServerSent.deserialize(rawArray);
      console.log(event.message);
      switch (event.message) {
        case "add": {
          const xd = members.find((x) => x[0] == event.add.name);
          if (!xd) {
            members.push([event.add.name, true, 0]);
          } else {
            xd[1] = true;
          }
          console.log(event.add.name);
          setMembers([...members]);
          console.log(members);
          break;
        }
        case "remove":
          members.find((x) => x[0] == event.remove.name)![1] = false;
          setMembers([...members]);
          break;
        case "new_problem":
          setProblem(event.new_problem.problem);
          break;
        case "end":
          navigate(`/logs/${lobbyId}`);
          break;
        case "score_update":
          members.find((x) => x[0] == event.score_update.name)![2] =
            event.score_update.score;
          setMembers([...members]);
          break;
        case "start":
          setGameTime({
            startTime: new Date(event.start.startTime.seconds),
            duration: event.start.duration.seconds,
          });
          break;
        case "wrong":
          break;
        default:
          console.log("");
      }
    };
  }, [loggedIn, loggedIn.otp]);

  const { register, handleSubmit } = useForm<RequestStartParams>();
  const onSubmit: SubmitHandler<RequestStartParams> = async (data) => {
    ws.current!.send(
      new ClientSent({
        request_start: new ClientSent.RequestStart({
          duration: new google.protobuf.Timestamp({ seconds: data.duration }),
          is_random: data.isRandom,
          problems: [],
        }),
      }).serialize()
    );
  };
  if (!loggedIn.otp) {
    return (
      <LoginForm
        lobbyId={lobbyId!}
        handleLoginResponse={(res) => {
          localStorage.setItem(lobbyId!, res.otp);
          localStorage.setItem(lobbyId! + "_is_owner", String(res.is_owner));
          setOtp(res);
        }}
      />
    );
  }

  return gameTime ? (
    <>
      <div>
        <Scoreboard
          score={score}
          timeLimit={gameTime.duration}
          gameOver={gameOver}
          newProblem={newProblem}
        />{" "}
        <br />
      </div>{" "}
      <br />
      <LatexBoxes
        goalText={
          problem ? problem : { latex: "\\LaTeX", description: "", title: "" }
        }
        correctCallback={(answer) => {
          setScore(score + problem!.latex.length);
          submitAnswer(answer!);
        }}
      />{" "}
      <br />
      <Leaderboard entries={members} />
    </>
  ) : (
    <>
      <div>
        Members: <LobbyMembers members={members} />
      </div>
      <div hidden={!loggedIn.is_owner}>
        {" "}
        Settings
        <form onSubmit={handleSubmit(onSubmit)}>
          {" "}
          Duration (seconds):{" "}
          <input
            type="number"
            defaultValue={120}
            {...register("duration", { required: true })}
          />{" "}
          <br />
          Random order:{" "}
          <input
            type="checkbox"
            defaultChecked
            {...register("isRandom", { required: true })}
          />{" "}
          <br />
          <input type="submit" />
        </form>
      </div>
    </>
  );
};

const MainPage = () => (
  <div
    style={{
      // display: "flex",
      // width: ",
      justifyContent: "center",
      alignItems: "center",
    }}
  >
    <p>
      <KaTeXWrapper
        text={"\\text{This is a game to test your }\\LaTeX\\text{ skills}"}
      />
    </p>
    <h3>Single Player</h3>
    <Button path="/solo/timed" text="Timed Mode" />
    <Button path="/solo/zen" text="Zen Mode" />
    <h3>Multi-player</h3>
    <p>
      Create a game and share the link with friends! <br />
    </p>
    <Button path="/multi/create" text="Create Lobby" />
  </div>
);

const router = createBrowserRouter([
  {
    path: "/",
    element: (
      <div style={{ textAlign: "center" }}>
        <MainPage />
      </div>
    ),
  },
  {
    path: "/solo/zen",
    element: <SinglePlayerLaTeX />,
  },
  {
    path: "/solo/timed",
    element: <SinglePlayerLaTeX timeLimit={180} />,
  },
  {
    path: "/multi/create",
    element: <CreateLobby />,
  },
  {
    path: "/game/:lobbyId",
    element: <Game />,
  },
]);

const Header = () => {
  return (
    <div style={{ textAlign: "center" }}>
      <h1>
        <KaTeXWrapper text={"\\text{🍴-}\\TeX\\text{nique}"} />
      </h1>
      <h2>
        <KaTeXWrapper text={"\\text{A }\\LaTeX\\text{ Typesetting Game}"} />
      </h2>
    </div>
  );
};

ReactDOM.createRoot(document.getElementById("root")!).render(
  <>
    <Header />
    <RouterProvider router={router} />
  </>
);
