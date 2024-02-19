import React from "react";
import { useState, useEffect } from "react";

const Countdown = React.memo(
  (props: { timeLimit?: number; overCallback?: () => void }) => {
    const [timeLeft, setTime] = useState(props.timeLimit);
    useEffect(() => {
      const interval = setInterval(() => {
        setTime((prevTime) => {
          if (prevTime === 0 && props.overCallback) {
            props.overCallback();
            return 0;
          }
          return prevTime ? prevTime - 1 : undefined;
        });
      }, 1000);

      return () => {
        clearInterval(interval);
      };
    }, [props, props.overCallback]);

    if (timeLeft === undefined) {
      return "∞";
    }

    const date = new Date(timeLeft * 1000);
    return (
      String(date.getMinutes()) +
      ":" +
      String(date.getSeconds()).padStart(2, "0")
    );
  }
);

export default Countdown;
