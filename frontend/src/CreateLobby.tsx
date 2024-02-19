import { useForm, SubmitHandler } from "react-hook-form";
import { useNavigate } from "react-router-dom";
import { makeProtoRequest, readProtoResponse } from "./helper";
import { CreateLobbyReq, CreateLobbyRes } from "./message_passing";
import { LaTeXButton } from "./styles";

type CreateLobbyInput = {
  name: string;
};

const CreateLobby = () => {
  const navigate = useNavigate();
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<CreateLobbyInput>();
  const onSubmit: SubmitHandler<CreateLobbyInput> = async (data) => {
    const req = new CreateLobbyReq({
      lobby_name: data.name,
    });

    const ret = await makeProtoRequest("/createLobby", req);
    const lobbyId = CreateLobbyRes.deserialize(
      await readProtoResponse(ret)
    ).lobby_id;
    navigate(`/game/${lobbyId}`);
  };

  return (
    <>
      <h3>Create Lobby</h3>
      <form onSubmit={handleSubmit(onSubmit)}>
        Name <input {...register("name", { required: true })} />
        {/* errors will return when field validation fails  */}
        {errors.name && <span>This field is required</span>} <br />
        <input style={LaTeXButton} type="submit" />
      </form>
    </>
  );
};

export default CreateLobby;
