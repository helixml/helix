import React from 'react';
import { styled } from '@mui/system';

const Container = styled('div')({
    maxWidth: '1200px',
    margin: '0 auto',
    padding: '20px',
});

const Header = styled('div')({
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    height: '100px',
});

const Block = styled('div')({
    display: 'flex',
    alignItems: 'center',
    padding: '40px 20px',
    marginBottom: '40px',
    boxShadow: '0 4px 8px rgba(0,0,0,0.1)',
});

const Media = styled('div')({
    flex: '1',
    paddingRight: '40px',
});

const Content = styled('div')({
    flex: '1',
    textAlign: 'left',
});

const Button = styled('button')({
    padding: '10px 20px',
    border: 'none',
    backgroundColor: '#007BFF',
    color: 'white',
    borderRadius: '4px',
    marginTop: '20px',
});

function App() {
    return (
        <Container>
            <Header>
                <img src="/img/helix-text-logo.png" alt="Helix Logo" style={{width:"250px"}} />
            </Header>
            <OpenAIBlock />
            <ImageModelsBlock />
            <LanguageModelsBlock />
            <DeploymentBlock />
            <Footer />
        </Container>
    );
}

function OpenAIBlock() {
    return (
        <Block>
            <Media>
                <video autoPlay loop muted style={{width:"300px"}}>
                    <source src="/img/typing.mp4" type="video/mp4"/>
                </video>
            </Media>
            <Content>
                <h2>Open AI 😉</h2>
                <p>Deploy the latest open source models securely into your cloud</p>
                <p>Or let us run them for you</p>
            </Content>
        </Block>
    );
}

function ImageModelsBlock() {
    return (
        <Block>
            <Media>
                <img src="/img/sdxl.png" alt="Stable Diffusion XL" style={{width:"300px"}} />
            </Media>
            <Content>
                <h2>Image models</h2>
                <p>Train your own SDXL customized to your brand or style</p>
                <Button>FINE TUNE SDXL</Button>
            </Content>
        </Block>
    );
}

function LanguageModelsBlock() {
    return (
        <Block>
            <Media>
                <img src="/img/mistral.png" alt="Mistral-7B" style={{width:"300px"}} />
            </Media>
            <Content>
                <h2>Language models</h2>
                <p>Small open source LLMs are beating proprietary models</p>
                <Button>FINE TUNE MISTRAL-7B</Button>
            </Content>
        </Block>
    );
}

function DeploymentBlock() {
    return (
        <Block>
            <Media>
                <img src="/img/servers.png" alt="Servers in a data center" style={{width:"300px"}} />
            </Media>
            <Content>
                <h2>Deployment</h2>
                <ul style={{ listStyleType: 'none', padding: '0' }}>
                    <li>GPU scheduler</li>
                    <li>Smart runners</li>
                    <li>Autoscaler</li>
                </ul>
                <Button>DEPLOY ON YOUR INFRA</Button>
            </Content>
        </Block>
    );
}

function Footer() {
    return (
        <Block>
            <Media>
                <img src="/img/github.png" alt="GitHub users collaborating" style={{width:"250px"}} />
            </Media>
            <Content>
                <h2>Clone us on GitHub</h2>
                <p>Bring new models to the open stack</p>
                <a href="https://github.com/helix-ml/helix" style={{ display: 'block', marginBottom: '20px' }}>GITHUB.COM/HELIX-ML/HELIX</a>
                <Button>JOIN MLOPS.COMMUNITY SLACK</Button>
            </Content>
        </Block>
    );
}

export default App;
