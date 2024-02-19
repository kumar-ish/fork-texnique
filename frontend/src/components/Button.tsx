import { useNavigate } from "react-router-dom";
import { LaTeXButton } from "../styles";

function Button(props: { path: string; text: string }) {
  const navigate = useNavigate();
  function handleClick() {
    navigate(props.path);
  }

  return (
    <button type="button" style={LaTeXButton} onClick={handleClick}>
      {props.text}
    </button>
  );
}

export default Button;
