import { useForm, SubmitHandler } from "react-hook-form";
import { makeProtoRequest, readProtoResponse } from "./helper";
import { LoginResponse, LoginRequest } from "./message_passing";

type LoginInput = {
  username: string;
  password: string;
};

const LoginForm = (props: {
  lobbyId: string;
  handleLoginResponse: (arg0: LoginResponse) => void;
}) => {
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginInput>();
  const onSubmit: SubmitHandler<LoginInput> = async (data) => {
    await makeProtoRequest(
      "/login",
      new LoginRequest({ ...data, lobby_id: props.lobbyId })
    )
      .then(async (res) => {
        const message = LoginResponse.deserialize(await readProtoResponse(res));
        props.handleLoginResponse(message);
      })
      .catch((e) => {
        alert(e);
      });
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)}>
      Username: <input {...register("username", { required: true })} /> <br />
      {errors.username && <span>This field is required</span>}
      Password:{" "}
      <input
        type="password"
        {...register("password", { required: true })}
      />{" "}
      <br />
      <input type="submit" />
    </form>
  );
};

export default LoginForm;
